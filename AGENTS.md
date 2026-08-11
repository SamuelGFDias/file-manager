# AGENTS.md — file-manager

Um conjunto de ferramentas de linha de comando para gerenciar, manipular e organizar arquivos PDF com precisão e automação. Desenvolvido em Go puro (`CGO_ENABLED=0`), roda em Windows e Linux como um único binário, sem dependências externas.

## Visão Geral

- **Módulo:** `github.com/SamuelGFDias/file-manager`
- **Go:** 1.26.5
- **Binário:** `file-manager` (entrypoint: `cmd/file-manager`)
- **Ferramentas:** `merge-pdf`, `split-pdf`, `organize-pdf`, `ocr-pdf`
- **Libs principais:** cobra (CLI), survey/v2 (prompts), yaml.v3 (perfis), pdfcpu (manipulação), ledongthuc/pdf (extração de texto), fatih/color, mattn/go-isatty

## Estrutura de Pastas

```
cmd/file-manager/                 Entrypoint — main() roda app.Execute()
cmd/scaffold/                      Gerador de novas ferramentas
internal/app/                      Registry de ferramentas + montagem do cobra
internal/ui/                       Screen, Navigator, Clear (abstração cross-platform)
internal/ui/filepicker/           Seleção interativa de arquivos/pastas
internal/ui/calibrate/            Calibração interativa de regex
internal/ui/profiles/             CRUD genérico de perfis YAML (reutilizado por todas ferramentas)
internal/ui/undo/                 Tela interativa de desfazer uma organização
internal/ui/mainmenu/             Menu principal
internal/ui/docs/                 Exportação de documentação (context e skill)
internal/commanddocs/             Doc dos comandos que NÃO são ferramentas do registry (undo, profiles, update, version, docs export)
internal/tool/                    Contrato Tool/Param/Doc
internal/config/                  Gerenciamento de perfis YAML (paths, validação, I/O)
internal/history/                 Manifesto de operações reversíveis (organize-pdf) + lógica de desfazer (history.Undo)
internal/pdfutil/                 Núcleo: merge, split, organize, extração de texto (com fallback OCR)
internal/ocr/                     Wrapper do executável externo tesseract (não é binding CGO)
internal/regexcalib/              Sugestão de regex a partir de valor de exemplo
internal/selfupdate/              Auto-atualização: consulta release, compara versão, baixa e substitui o executável
internal/tools/                   Uma subpasta por ferramenta (mergepdf/, splitpdf/, organizepdf/, ocrpdf/)
internal/testcli/                 Harness de teste ponta a ponta: abre o binário real num pty (linux only)
e2e/                               Cenários de teste ponta a ponta (tag "e2e"; rodam via "make e2e")
```

## Decisões de Arquitetura

### 1. Declaração Única de Parâmetro

Cada parâmetro de ferramenta é declarado **uma única vez** como `tool.Param`. Disso derivam três coisas automaticamente:

- **Flag real do cobra** (via `BindFlag`, um callback que chama `fs.StringVarP(...)`)
- **Pergunta interativa** (via `Prompt`, outro callback que usa survey)
- **Documentação exportável** (via `tool.DocFlags`)

A duplicação dessas três é raiz de bugs: documentação diverge da flag real, ou a pergunta não corresponde ao que a flag faz. Solução: nunca chamar `cobra.Flags().StringVar()` diretamente. Sempre declarar via `tool.Param` em `params()` e depois chamar `tool.BindAll(fs, params())`.

Existe teste `TestToolsConsistency` em `internal/app/consistency_test.go` que valida a consistência entre documentação e flags. O teste roda sobre todas as ferramentas devolvidas por `app.Tools()` (um subteste por ferramenta) e verifica: (a) toda flag documentada em `Doc().Flags` existe de fato no `Command()`; (b) toda flag real registrada no `Command()` está documentada; (c) valor default documentado bate com o default real da flag; (d) shorthand documentado bate com o real; (e) `Meta().ID` corresponde a `Use` do comando, e `Doc().ID` a `Meta().ID`; (f) campos essenciais da `Doc` (Title, Summary, Description) não estão vazios e há pelo menos um exemplo; (g) todo comando de exemplo começa com `file-manager <id-da-ferramenta>`. Detalhe importante: como esse teste itera `app.Tools()`, uma ferramenta nova **entra na cobertura automaticamente** ao ser registrada em `internal/app/registry.go` — ninguém precisa lembrar de escrever esse teste por ferramenta.

**Onde declarar params:**
- Ferramenta implementa `params() []tool.Param`
- Tela chama `tool.PromptAll(params())` para fazer todas as perguntas
- Command cobra chama `tool.BindAll(fs, params())` para registrar as flags

### 2. Padrão de Telas

Toda tela interativa implementa `ui.Screen`:

```go
type Screen interface {
    Title() string          // Nome para breadcrumb
    Run(nav *Navigator) error // Executa lógica e navega
}
```

O `ui.Navigator` mantém uma pilha de telas. A cada iteração do loop principal:
1. Tela do topo limpa o terminal
2. Desenha breadcrumb com `Title()` de cada tela na pilha
3. Chama `Run(nav)` da tela do topo
4. Tela chama `nav.Push()`, `nav.Pop()`, `nav.Replace()` ou `nav.Exit()` para navegar

**Importante:** O arquivo `screen.go` de cada ferramenta é **apenas cola de I/O**. Não contém lógica de negócio. Não é testado linha a linha — testes testam lógica pura em pacotes de domínio.

### 3. Lógica Pura Separada da Interface

A lógica de negócio vive em pacotes de domínio (ex: `internal/pdfutil/`, `internal/regexcalib/`):

- **100% testável sem terminal, sem arquivo real ou sem PDF**
- Testes usam **testes de tabela** (`table-driven tests`)
- A tela é apenas um wrapper que coleta inputs do usuário e chama a função de domínio

Exemplo:
- `pdfutil.Merge()` → função pura que une PDFs
- `internal/tools/mergepdf/screen.go` → pergunta inputs, valida, chama `pdfutil.Merge()`

### 4. Dry-run Compartilhado

Não há duas implementações de uma operação. O `--dry-run` de `organize-pdf` chama exatamente a mesma função `pdfutil.Organize()` com `DryRun: true`. A tela de calibragem (modo interativo) também chama com `DryRun: true`.

Resultado: a simulação **nunca pode divergir** do que a execução real faria.

### 5. Perfis (Configurações Salvas)

Um perfil é um arquivo YAML reutilizável que armazena as opções de uma ferramenta:

- **Windows:** `%AppData%\file-manager\profiles\<ferramenta>\<nome>.yaml`
- **Linux/macOS:** `~/.config/file-manager/profiles/<ferramenta>/<nome>.yaml`

**Registro de perfil:**
```yaml
name: meu-perfil
tool: merge-pdf
created_at: 2025-08-11T...
updated_at: 2025-08-11T...
data:
  inputs:
    - /path/to/folder
  max_depth: 1
  sort: name
  overwrite: false
```

Validação: `config.ValidateName()` rejeita nomes com `/`, `\`, `..`, garantindo proteção contra path traversal.

**Reutilização:** Uma única tela genérica (`internal/ui/profiles/`) serve todas as ferramentas — nenhuma reimplementa CRUD. Ferramentas só precisam implementar `tool.ProfileSupport` (interface com `Empty()`, `Edit()`, `Apply()`).

**Exportação e importação (`file-manager profiles export`/`import`):** o perfil calibrado nasce e ficava preso ao diretório de configuração da máquina onde foi criado — sem nenhum comando para tirá-lo de lá. Isso importa porque quem calibra as regras (o dono do projeto) e quem usa a ferramenta no dia a dia (alguém sem familiaridade com regex, em outra máquina) costumam ser pessoas diferentes. `config.ExportProfile(toolID, name, destPath)` grava exatamente o mesmo envelope `config.Profile` usado internamente (`name`, `tool`, `created_at`, `updated_at`, `data`) num arquivo externo — **decisão deliberada:** não existe um formato paralelo de exportação para manter sincronizado com o envelope interno; exportar e importar são simétricos por construção (`ExportProfile` grava o mesmo `Profile` que `ReadProfileFile` lê de volta). `config.ReadProfileFile(path)` valida a estrutura do arquivo (YAML válido, `tool` e `name` preenchidos, `name` aprovado por `ValidateName`) mas **não** decodifica `data` contra nenhuma struct — só quem chama (o comando `profiles import` em `internal/app/root.go`, ou a tela em `internal/ui/profiles/screen.go`) conhece o registro de ferramentas e pode resolver `tool.Profile().Empty()` para validar o decode. Essa validação do conteúdo acontece **na importação**, de propósito: um arquivo corrompido ou de uma versão incompatível do CLI precisa falhar ali, não no meio de um lote de arquivos processado silenciosamente com dados errados. `config.ImportProfile(p, name, overwrite)` valida o nome de destino com `ValidateName` (pode ser diferente do nome original do arquivo — quem importa decide), respeita `overwrite` (erro se já existir e `overwrite=false`) e preserva `created_at` de um perfil já existente que está sendo sobrescrito, seguindo a mesma lógica de `Save`. Subcomandos: `list [--tool]` (agrupado por ferramenta quando `--tool` não é informado), `export --tool --name --output` (as três obrigatórias), `import --file [--name] [--force]`, `path` (imprime o diretório de perfis, para quem não sabe o que é `os.UserConfigDir`). A tela interativa ganhou as ações "Exportar para arquivo" e "Importar de arquivo" no mesmo menu de ações por ferramenta.

### 6. Sanitização de Nome de Arquivo

Nomes derivados de captura de regex (ex: `{{.Match}}` em split-pdf) passam por `pdfutil.SanitizeFilename()`:

- Remove separadores de caminho (`/`, `\`)
- Remove `..`
- Remove caracteres inválidos no Windows (`:`, `*`, `?`, `"`, etc.)

Defesa contra PDF malicioso escrever fora do diretório de destino.

### 7. OCR como Fallback de Extração de Texto

A extração de texto (`pdfutil.ExtractText()` / `ExtractPageTextsOpts()`) usa `ledongthuc/pdf` para PDFs com camada de texto. PDFs digitalizados (imagem pura) devolvem texto vazio nesse caminho — mas a página já é uma imagem embutida, então o pdfcpu a extrai e o `internal/ocr.Tesseract` a lê. Não é preciso rasterizador.

Controlado por `pdfutil.OCRMode` (`auto`/`always`/`never`) dentro de `pdfutil.TextOptions`, exposto em `split-pdf` e `organize-pdf` pelas flags `--ocr` (default `auto`) e `--ocr-lang` (default `por`), persistidas no perfil YAML como `ocr` e `ocr_lang`. `auto` só aciona o OCR quando a página não tem texto embutido.

Texto extraído por OCR é cacheado por arquivo (`ParseExtractedImageName` mapeia a imagem extraída de volta à página) — o OCR custa ~1s por página, então repetir a extração para a mesma página seria caro.

Sem o Tesseract instalado, nada quebra: a execução segue normalmente e emite um aviso com `ocr.InstallHint()` (instrução de instalação por sistema operacional).

**O OCR erra caracteres** (observado: `ESCOLA` → `ESCO`, confusão entre `0` e `O`). Regex sobre conteúdo potencialmente vindo de OCR devem ser tolerantes a esse tipo de erro, não exigir casamento exato.

**Armadilha corrigida na v0.8.1 — o pdfcpu não nomeia a imagem extraída sempre com o prefixo `Im`.** `api.ExtractImagesFile` grava cada imagem como `<baseDoPDF>_<página>_<prefixo><índice>.<ext>`, e esse prefixo vem do **nome do recurso XObject da página** (`WriteImageToDisk` em `pdfcpu/pkg/api/extract.go`, campo `img.Name`) — não é uma convenção fixa do pdfcpu, varia conforme como o PDF de origem nomeou o XObject. `ParseExtractedImageName`/`extractedImageNamePattern` (`internal/pdfutil/textextract.go`) travavam nesse prefixo sendo `Im`; num PDF real de duas páginas a segunda saiu nomeada `X0`, e sua imagem nunca chegava ao Tesseract. O padrão foi generalizado para aceitar qualquer prefixo de letras (`[A-Za-z]+`) antes do índice numérico.

O defeito de fundo não era a expressão regular, e sim que um arquivo extraído não reconhecido era **descartado sem avisar ninguém** — foi esse silêncio, não a regex em si, que permitiu o bug atravessar seis versões (v0.2.0 → v0.8.0) sem ser notado: o usuário via "nenhum texto foi extraído" ou a regex do usuário não casando, sem nenhuma pista de que a causa era um arquivo de imagem ignorado. Por isso, além de aceitar qualquer prefixo, `ExtractPageTextsOpts`/`ExtractTextOpts` agora devolvem um segundo valor (`[]string` de avisos não-fatais): um aviso por arquivo extraído cujo nome não bate com o padrão (citando o nome), e um aviso agregado à parte quando imagens foram extraídas mas **nenhuma** pôde ser associada a uma página — sinal forte de que o pdfcpu mudou de novo a convenção de nomes. Esses avisos são propagados por quem chama até `OrganizeResult.Warnings`/`SplitResult.Warnings` (prefixados com o nome do PDF de origem em `Organize`, já que ali um lote processa vários arquivos). Se o pdfcpu mudar a nomenclatura outra vez no futuro, o efeito passa a ser um aviso visível, nunca mais uma página sumindo em silêncio — **não reintroduzir um `continue` sem aviso nesse laço.**

### 8. Registro de Ferramenta Nova

O gerador (`make new-tool NAME=x`) cria o esqueleto em `internal/tools/x/`, mas o registro é um passo **deliberadamente manual** de uma linha em `internal/app/registry.go`:

```go
func Tools() []tool.Tool {
    return []tool.Tool{
        mergepdf.New(),
        splitpdf.New(),
        organizepdf.New(),
        mynewtoolutool.New(),  // ← linha manual
    }
}
```

Isso é decisão deliberada: editar Go existente por parsing automático é frágil. O ganho não compensa o risco.

### 9. OCR via Processo Externo, não CGO

`internal/ocr` invoca o executável `tesseract` via `os/exec` em vez de usar um binding CGO (ex: `gosseract`). Motivo: um binding CGO exigiria `CGO_ENABLED=1` e a libtesseract instalada no ambiente de build, o que quebraria a distribuição atual — binário único, estático, cross-compilado de Linux para Windows sem toolchain C. Com processo externo, o binário Go continua exatamente como antes; o Tesseract é uma dependência de runtime opcional, não de build.

### 10. Calibração e Processamento Compartilham `TextOptions`

Em `organize-pdf`, a tela de calibração interativa (que sugere regex a partir de um exemplo) e o processamento real usam o mesmo `pdfutil.TextOptions` — mesmo modo de OCR, mesmo idioma. Sem isso, o usuário calibraria a regex contra um texto (ex: extração nativa) e a execução real veria outro (ex: texto de OCR), e a regex calibrada poderia não casar. Mesmo princípio da Decisão 4 (dry-run compartilhado): não há dois caminhos que possam divergir silenciosamente.

### 11. Validação Antecipada e Encadeamento de Seletores em `organize-pdf`

Dois defeitos de usabilidade relatados por uso real (corrigidos na v0.2.1), ambos no fluxo interativo de `organize-pdf` (`internal/tools/organizepdf/screen.go`):

**Pasta de origem vazia só era percebida no final.** O usuário selecionava uma pasta sem PDFs, percorria toda a calibração de regex (níveis, nome do arquivo, teste) e só então via "0 de 0 arquivos". Agora `pickInputDir()` conta os PDFs (`countPDFs()`) no ato da seleção: zero PDFs bloqueia o avanço e oferece escolher outra pasta, com limite de `maxSourceDirAttempts` (5) tentativas. Com PDFs, confirma imediatamente ("N PDFs encontrados na pasta de origem."). `pickSample()` também avisa quando o PDF de amostra está **fora** da pasta de origem (`sampleOutsideInput()`) e pede confirmação explícita (default: não, `maxSampleAttempts` = 5 tentativas) — foi assim que o usuário calibrou contra um documento que não fazia parte do lote. Antes de aplicar, `showConfigSummary()` mostra caminhos absolutos de origem/destino, contagem de PDFs e se vai copiar ou mover.

**Cada seletor reabria no diretório do executável.** Depois de escolher `~/Downloads` como origem, o prompt de destino reabria em `~/.file_manager` (pasta do binário) — sem subpastas, parecia vazia, e o usuário achava que a seleção não tinha funcionado. Correções:

- Em `configure()`, o prompt de destino agora começa em `inputDir` (a pasta de origem recém-selecionada), não em `"."`.
- `internal/ui/filepicker/filepicker.go` ganhou memória de pacote do último diretório usado com sucesso: `LastDir()`, `ResetLastDir()` (só para isolar testes) e `resolveStart(start string)`.

  **Regra de precedência do `resolveStart` — não inverter:** um `start` explícito e não-vazio **sempre vence** sobre a memória; `LastDir()` só é consultado quando o chamador passa `start == ""`. Isso é proposital: um fluxo que encadeia seletores passando o diretório anterior como `start` (como `organize-pdf` faz) precisa que esse valor seja respeitado à risca, e `splitpdf`/`mergepdf` continuam passando `"."` explicitamente — se a memória pudesse sobrescrever um `start` explícito, o comportamento desses dois viraria imprevisível sem que ninguém tivesse pedido a mudança. Um futuro ajuste que "simplifique" trocando a ordem de checagem quebra essa garantia silenciosamente.

- Novas funções `PickFileWithPrompt(start, prompt, exts)` / `PickDirWithPrompt(start, prompt)` aceitam uma mensagem específica de contexto (`PASTA DE ORIGEM`, `PASTA DE DESTINO`, `PDF de AMOSTRA`) em vez do genérico "Selecione um diretório"; `PickFile`/`PickDir` mantiveram as assinaturas antigas e passaram a delegar para as novas com uma mensagem padrão.

### 12. Auto-atualização (`internal/selfupdate`)

O usuário final não acompanha o repositório, então o subcomando `file-manager update` é o único caminho de atualização. Decisões relevantes para quem mexer em `internal/selfupdate` ou no aviso do menu principal:

- **Checagem em segundo plano, uma vez por sessão, falha silenciosa.** `selfupdate.Checker` dispara a consulta ao GitHub em goroutine (`Start()`, idempotente via `sync.Once`) no momento em que o menu principal é construído (`mainmenu.NewScreen`). Qualquer erro — rede indisponível, limite de requisições da API, versão local não-semver (`dev`) — termina em silêncio, sem preencher o aviso: o usuário não veio até o menu para depurar conectividade. Uma checagem síncrona travaria a abertura do menu; uma checagem a cada redesenho da tela bateria no limite de requisições da API do GitHub.
- **`WaitNotice(timeout)` existe porque uma consulta puramente não-bloqueante perde a corrida contra a rede na primeira abertura (corrigido na v0.3.1).** `Notice()` só lê o resultado já pronto e devolve `("", false)` na hora se a checagem em segundo plano (disparada ~250ms antes, em média, pela chamada à API do GitHub) ainda não terminou — e como o `survey.Select` assume o terminal logo depois e o menu não é redesenhado enquanto o usuário navega, na prática o aviso quase nunca aparecia na primeira abertura, só se o usuário entrasse numa ferramenta e voltasse ao menu. `WaitNotice(timeout)` aguarda **no máximo** o timeout pelo resultado antes de desistir; `mainmenu.screen.Run` paga essa espera (1,5s) só na primeira renderização (`firstRender`), e usa `Notice()` — que continua não-bloqueante, sem mudança de contrato — em todas as renderizações seguintes, já que o resultado já estará pronto. Sem internet, ou se o timeout estourar, o menu abre normalmente e nada é exibido; build local (`dev`) não espera nada, pois `WaitNotice` devolve na hora quando a versão local não é semver.
- **Binário baixado é validado por execução antes da troca.** `VerifyBinary` roda `<binário-baixado> version` e falha se o processo não sair limpo ou não produzir saída — sinal de download truncado/corrompido. Só depois disso `ReplaceExecutable` é chamado; um download corrompido nunca chega a substituir o executável em uso.
- **Substituição via rename, com tratamento especial no Windows.** `ReplaceExecutable`/`replaceAt` usa `os.Rename` por cima do executável atual — em Linux/macOS isso funciona mesmo com o processo em execução (o processo antigo continua servindo o inode até terminar). No Windows não é possível sobrescrever nem apagar um `.exe` em execução, então `replaceWindows` renomeia o executável atual para `<nome>.old`, move o novo binário para o lugar dele e tenta apagar o `.old` (best-effort — o Windows só libera o arquivo depois que o processo antigo terminar). Se mover o novo binário falhar, o `.old` é restaurado para o nome original: deixar o usuário sem executável nenhum é o pior resultado possível.
- **`LatestRelease`/`Download` só aceitam HTTPS.** Baixar e depois executar um binário recebido por canal não autenticado seria um vetor óbvio de comprometimento; `Download` rejeita qualquer URL que não comece com `https://`.

**Severidade do aviso (`Severity`, `ClassifyUpdate`, `NoticeText`), a partir da v0.10.0: o aviso não pode tratar "correção de defeito" igual a "novidade".** Antes desta versão, qualquer atualização disponível gerava o mesmo texto ("nova versão disponível"). Isso é enganoso quando o motivo de atualizar é sério: a v0.8.1 corrigiu um defeito em que o OCR ignorava páginas silenciosamente (perfis de organização baseados em texto extraído por OCR perdiam classificações sem nenhum aviso), e quem ficasse na v0.8.0 continuaria produzindo resultado incompleto sem saber que existia correção.

- **`Releases(ctx, repo)` substitui `/releases/latest` por `/releases?per_page=100`** no fluxo de verificação (`Checker` e o comando `update`), descartando rascunhos e pré-lançamentos. Continua sendo uma única requisição — não há custo adicional de limite de API — mas agora `ClassifyUpdate` enxerga **todo o caminho** de versões entre a atual e a mais recente, não só o topo. `LatestRelease` (`/releases/latest`) continua existindo no pacote, sem uso no fluxo de verificação a partir desta versão (mantida por ser uma API pública testada e potencialmente útil para quem só quer o topo).
- **`ClassifyUpdate(current, releases) (latest Release, sev Severity, ok bool, err error)` é o núcleo, e a regra central é sutil: a severidade NÃO é decidida comparando `current` só contra a versão mais recente — ela varre TODOS os releases mais novos que `current`.** Motivo, com números concretos: se `current` é `0.8.0` e a mais recente publicada é `0.9.0`, o salto por si só parece um bump de minor (só novidade). Mas se `0.8.1` foi publicado no meio (`0.8.0 < 0.8.1 < 0.9.0`), a correção de `0.8.1` **já está incluída** em `0.9.0` — releases são cumulativos — e quem está em `0.8.0` está, agora, exposto ao defeito que `0.8.1` corrigiu. Comparar só "atual" contra "mais recente" perderia esse fato e mostraria "novidade" para quem precisa de "correção". **Se um refactor futuro "simplificar" isso para um `CompareVersions(current, latest.TagName)` direto (como o código fazia antes desta versão), o caso `0.8.0 → 0.9.0` com `0.8.1` no meio volta a ser classificado como novidade — reintroduzindo exatamente o problema que motivou esta mudança.** Precedência das regras: `current` não-semver → `ok=false` sem erro (build local não é comparável); nenhum release mais novo → `ok=false`, `SeverityNone` (já atualizado, ou build de desenvolvimento à frente de tudo publicado); algum release mais novo com major maior → `SeverityMajor` (vence mesmo que também haja correção no caminho — mudança incompatível é o risco mais sério, pois pode quebrar automação de quem já usa o formato/flags atuais); senão, algum release mais novo com patch > 0 → `SeverityPatch`; senão → `SeverityMinor`. `latest` devolvido é sempre o release de maior versão entre os mais novos, independente de qual causou a severidade (no exemplo acima, `latest` é `0.9.0`, não `0.8.1`).
- **`NoticeText(current, latest, sev)` é função pura**, extraída para ser testável sem rede nem terminal: monta o texto em português para cada severidade, sempre com as duas versões visíveis. `SeverityPatch` menciona explicitamente "correção" e o risco de resultado inconsistente continuando na versão atual; `SeverityMajor` menciona "incompatibilidade" e inclui `latest.HTMLURL`, recomendando ler as notas do release antes de atualizar.
- **No menu principal e no comando `update`, `ui.Warnf` é usado para `SeverityPatch` e `SeverityMajor`; `ui.Infof` para `SeverityMinor`.** Novidade pura não precisa da mesma urgência visual que correção de defeito ou mudança incompatível. `Checker.Notice()`/`WaitNotice()` mantiveram a assinatura `(string, bool)` — a severidade é consultada à parte via `Checker.Severity()`, para não quebrar quem só queria o texto.

**A partir da v1.0.0, `--version`/`-v` e o subcomando `version` também emitem o aviso de atualização — antes só o menu interativo o fazia.** Motivação: quem digita `--version` está perguntando exatamente "em que versão estou?", e é o momento natural para saber que existe uma mais nova, sobretudo quando ela corrige um defeito. O desenho precisou resolver duas restrições não-negociáveis ao mesmo tempo, e a mesma decisão resolve as duas:

- **A supressão do aviso é decidida por `ui.IsOutputTerminal()` (stdout), não por `ui.IsInteractive()` (stdin).** São perguntas diferentes: `IsInteractive()` decide se o MENU pode abrir (por isso olha stdin — sem stdin de terminal não há como o usuário navegar um `survey.Select`). Aqui a pergunta é "alguém está olhando a saída agora, ou ela está sendo redirecionada/canalizada?" — e `file-manager --version > arquivo` mantém o stdin ligado ao terminal de quem digitou o comando (`IsInteractive()` continuaria `true`) enquanto o stdout vai para um arquivo. Decidir por stdin faria exatamente o caso que mais importa proteger (`--version` redirecionado ou em pipe) ganhar uma linha extra que hoje não existe, quebrando o contrato de saída idêntica das três formas e qualquer script que capture `--version`. `IsOutputTerminal()` (`internal/ui/term.go`) é a mesma checagem via `isatty`, só que sobre `os.Stdout.Fd()` em vez de `os.Stdin.Fd()`.
- **`ui.WarnStderrf`/`ui.InfoStderrf` (novas variantes de `ui.Warnf`/`ui.Infof` que escrevem em `os.Stderr` em vez de `os.Stdout`) imprimem o aviso, nunca `ui.Warnf`/`ui.Infof`.** A linha de versão sempre sai primeiro em stdout, exatamente como hoje; o aviso — quando existe — vai inteiramente para stderr. Isso garante que capturar só stdout (`versao=$(file-manager --version)`, ou mesmo alguém em um terminal de verdade que redirecione só o stdout) nunca traz o aviso misturado ao valor da versão, e reforça (em vez de depender só de `IsOutputTerminal()`) a separação entre "resposta pedida" e "aviso extra".
- **O timeout de espera pela checagem (`versionNoticeTimeout`, em `internal/app/root.go`) é 1s — bem menor que o 1,5s do menu (`updateNoticeWaitTimeout`, `internal/ui/mainmenu`).** Quem abre o menu já aceitou uma tela cheia de I/O; quem digita `--version` fez uma pergunta pontual e espera resposta imediata — o aviso é um extra sobre essa resposta, nunca pode ser o motivo de fazer o comando demorar perceptivelmente. Estourado o timeout (sem rede, rede lenta), nada além da versão é impresso: mesmo silêncio absoluto do `Checker` já documentado acima.
- **A garantia de saída idêntica entre `--version`, `-v` e `version` (testada em `internal/app/version_flag_test.go`) continua valendo — só que agora explicitamente restrita a STDOUT.** As três formas chamam a mesma função (`printVersion`, em `internal/app/root.go`), o que torna a igualdade estrutural em vez de uma coincidência mantida por disciplina entre três blocos de código separados. Como o aviso vive em stderr e só aparece com `IsOutputTerminal()` verdadeiro, e os testes de unidade capturam stdout via `os.Pipe` (que nunca é um terminal), nenhum teste existente precisou de exceção — eles continuam comparando exatamente o que sempre compararam.
- **Consequência de arquitetura: `root.Version` (campo do `cobra.Command`) ficou vazio de propósito.** O mecanismo embutido do cobra (`Command.execute`) intercepta `--version` e retorna ANTES de qualquer `RunE` rodar, quando `c.Version != ""` — o mesmo mecanismo que intercepta `--help`. Isso bastava enquanto `--version` só precisava imprimir uma linha, mas não dá nenhum gancho para código rodar depois da impressão. Por isso o tratamento de `--version`/`-v` foi movido para dentro do `RunE` do comando raiz (que lê a flag manualmente via `cmd.Flags().GetBool("version")`) e `root.Version`/`root.SetVersionTemplate` foram removidos — a flag continua registrada manualmente (`root.Flags().BoolP("version", "v", ...)`, como já era) independente do valor de `root.Version`, então nada do comportamento de `--help` ou da tradução em português é afetado.
- **`printVersion(v Version, outputIsTerminal bool, newChecker func(repo, currentVersion string) *selfupdate.Checker)` recebe `newChecker` como parâmetro** especificamente para ser testável sem rede: os testes provam que, com `outputIsTerminal=false`, a factory nunca é chamada (contando invocações) — nem o `Checker` chega a ser criado, então `Start()` nunca dispara a consulta em segundo plano. Em produção, as duas chamadas (RunE da flag, e `newVersionCommand`) passam `selfupdate.NewChecker`, o construtor real.

### 13. Tema do `survey` Sobrescrito em `internal/ui`

`internal/ui/prompt.go` sobrescreve `survey.SelectQuestionTemplate` e `survey.MultiSelectQuestionTemplate` (função `ApplyTheme`, chamada uma única vez — idempotente via `sync.Once` — a partir de `mainmenu.NewScreen`, o ponto de entrada de toda sessão interativa) para ajustes de apresentação:

1. A descrição de uma opção de `survey.Select` só aparece quando ela é a opção **atualmente selecionada** (acompanha a seta), em vez de todas as opções ao mesmo tempo.
2. A dica em inglês `[Use arrows to move, type to filter]` (Select) foi traduzida para `[use ↑ ↓ para navegar, digite para filtrar, Enter para confirmar]`.
3. A dica em inglês `[Use arrows to move, space to select, ...]` (MultiSelect) foi traduzida para `[use ↑ ↓ para navegar, ESPAÇO para marcar, → marca todos, ← desmarca todos, digite para filtrar, Enter para confirmar]`.

**Regra geral, para não repetir o defeito abaixo:** todo template do `survey` que este CLI de fato usa precisa estar traduzido — não só o mais visível. `survey.Select` e `survey.MultiSelect` são os dois usados em telas de ponta a ponta (o segundo, via `filepicker.PickFiles`, para marcar vários arquivos); `survey.Confirm`/`survey.Input` também são usados, mas o único texto em inglês que os templates originais deles carregam fica atrás de `{{if .Help}}`/`{{if .Suggest}}`, e nenhuma pergunta deste programa define `Help` ou `Suggest` — esse texto nunca chega a ser exibido, então não há nada para traduzir ali. `survey.Password` não é usado em lugar nenhum do CLI. Ao adicionar um novo tipo de pergunta do `survey` (ou ao passar a usar `Help`/`Suggest` num já existente), reconferir se o template correspondente tem texto em inglês que passaria a ser exibido de verdade.

**Defeito real que motivou a regra acima (corrigido nesta versão, junto com a Decisão 19):** a tradução do template de `survey.Select` cobriu a escolha única desde a v0.2.1, mas a de `survey.MultiSelect` — usada por `filepicker.PickFiles` para marcar vários arquivos de uma vez — ficou de fora e permaneceu em inglês até `ocr-pdf` ser lançado (v0.11.0) e alguém tentar usá-la de verdade. A consequência não foi só estética: em `survey.MultiSelect`, **navegar é com as setas, mas MARCAR uma opção é com a barra de espaço — Enter só confirma o que já estiver marcado**. Num programa inteiramente em português, a única informação que explicava isso («space to select») estava em inglês. Um usuário navegou até uma pasta com PDF, apertou Enter sem marcar nada, e a seleção vazia resultante só foi percebida seis perguntas depois (ver Decisão 19) — o defeito de tradução e o de validação tardia se somaram no mesmo relato.

**Importante para quem for mexer nesses templates:** `selectQuestionTemplatePT`/`multiSelectQuestionTemplatePT` em `internal/ui/prompt.go` são **cópias adaptadas** dos templates padrão da biblioteca (`survey.SelectQuestionTemplate`/`survey.MultiSelectQuestionTemplate`, survey v2.3.7, `select.go`/`multiselect.go`), não implementações próprias. Qualquer atualização da dependência `survey` que mude os templates originais pode exigir portar a mudança manualmente para essas cópias — não há vínculo automático entre eles. Fora os ajustes de texto/apresentação acima, os templates foram mantidos idênticos aos originais de propósito: o objetivo é uma mudança cirúrgica de apresentação, não uma reescrita do comportamento do prompt.

### 14. Desfazer uma Organização (`internal/history`, comando `undo`)

`organize-pdf` copia por padrão (não destrutivo), mas `--move` move de verdade — e quem roda um lote grande com uma regex recém-calibrada erra pelo menos uma vez. A partir da v0.5.0, toda execução real (nunca uma simulação) grava um manifesto reversível, e `file-manager undo` desfaz a partir dele. Só funciona para operações feitas a partir desta versão — não há manifesto de organizações anteriores.

**Gravação injetada via `Recorder`, não acoplando `pdfutil` a `config`.** `internal/pdfutil.OrganizeOptions` ganhou um campo `Recorder func(action string, entries []RecordedEntry) error`. `internal/pdfutil.Organize` acumula uma `RecordedEntry` (caminhos absolutos de `Source`/`Dest`, mais `Size` do arquivo já no destino) por arquivo efetivamente copiado/movido — **incluindo os que foram para `--unclassified-dir`**, já que eles também precisam poder voltar — e, ao final de uma execução real com pelo menos uma entrada, chama `Recorder` uma única vez com todas elas. `Recorder` nil (o zero-value) desliga a gravação por completo: é o comportamento de quem chama `Organize` diretamente, inclusive os testes existentes de `internal/pdfutil`, que não precisaram mudar. `internal/tools/organizepdf/command.go` (`historyRecorder`) é o único ponto do CLI que conhece tanto `pdfutil` (o domínio de organização) quanto `internal/history` (o domínio de histórico) — nenhum dos dois pacotes de domínio importa o outro. **Decisão que não pode ser revertida silenciosamente:** se um refactor futuro tentar "simplificar" fazendo `pdfutil` chamar `history.Save` diretamente, isso reintroduz o acoplamento que esta injeção existe para evitar (e que o design original do projeto já evita entre `pdfutil` e `config`).

**Falha ao gravar o manifesto nunca falha a organização.** A operação de organizar já aconteceu de verdade quando `Recorder` é chamado; se ele devolver erro (ex: disco cheio ao gravar o YAML), `Organize` não propaga esse erro — anexa uma linha em `OrganizeResult.Warnings`, que o comando `organize-pdf` repassa em `tool.Result.Details`. Falhar uma operação já concluída, ou fingir que ela não aconteceu, seria pior do que simplesmente perder o histórico daquela execução específica.

**`internal/history`** não importa `internal/config` nem é importado por `internal/pdfutil`; repete o mesmo padrão de `config` (variável de pacote `userConfigDir`, substituível em teste) para resolver `<config>/file-manager/history`, em vez de importar `config` só para reaproveitar uma função de uma linha. A lógica de execução do desfazer (`history.Undo`, em `undo.go` do mesmo pacote) fica junto com a gravação do manifesto (`history.go`) — é regra de negócio pura, tão testável quanto o resto, e mantê-la no mesmo pacote evita um pacote extra que colidiria de nome com `internal/ui/undo` (a tela).

**As regras de segurança de `history.Undo` são o núcleo da feature, cada uma testada explicitamente** (`internal/history/undo_test.go`):

- **Nunca toca em um arquivo fora do manifesto.** `Undo` só olha para os caminhos em `Manifest.Entries` — nenhum outro arquivo da pasta de destino é sequer listado.
- **Verificação de tamanho antes de apagar ou mover, não de conteúdo.** Antes de tocar em `Entry.Dest`, `Undo` compara `os.Stat(...).Size()` com `Entry.Size` (gravado no momento da organização original). Tamanho diferente → a entrada é pulada (`SkipSizeChanged`), nunca apagada: o arquivo pode ter sido substituído ou editado depois. A verificação é por tamanho e não por hash/conteúdo de propósito — comparar o conteúdo inteiro de cada arquivo custaria caro demais num lote com centenas de PDFs, e o ganho de precisão não compensa esse custo recorrente.
- **Ação `copy`:** apaga `Entry.Dest`; `Entry.Source` nunca é lido nem tocado. **Ação `move`:** devolve `Entry.Dest` para `Entry.Source` via `os.Rename`, com fallback para copiar+remover (mesma estratégia de `moveOrCopyFile` em `pdfutil`, duplicada em `history` de propósito, para não acoplar os dois pacotes). Se `Entry.Source` já existir, a entrada é pulada (`SkipSourceExists`) — nunca sobrescreve.
- **Remoção de diretórios vazios, nunca recursiva.** Depois de restaurar/apagar todos os arquivos elegíveis, `removeEmptyDirsUpward` sobe de cada `Entry.Dest` até (sem incluir) `Manifest.OutputDir`, removendo um diretório só quando `os.ReadDir` confirma que está vazio — um diretório com qualquer arquivo estranho dentro é preservado.
- **Um manifesto já desfeito não pode ser desfeito de novo** sem `--force`: `Undo` devolve `ErrAlreadyUndone` (checável com `errors.Is`) quando `Manifest.UndoneAt != nil` e `force == false`, antes de tocar em qualquer arquivo. Quem chama `Undo` (comando `undo` e a tela `internal/ui/undo`) é responsável por chamar `history.MarkUndone(id, time.Now())` depois de uma execução real bem-sucedida — `Undo` em si não grava nada em disco além dos arquivos movidos/apagados.

**Comando `undo` e tela interativa compartilham `history.Undo`, nunca podem divergir sobre o que "desfazer" significa** — mesmo princípio da Decisão 4 (dry-run compartilhado em `organize-pdf`). `internal/app/undo.go` resolve qual manifesto usar (`--id` > `--last` > `survey.Select` em terminal interativo > erro claro pedindo uma das flags fora de terminal interativo), monta o plano com `history.Undo(m, dryRun=true, force)` (usado tanto para a mensagem informativa quanto para a confirmação — "afetando N arquivos"), e só então executa de verdade. `internal/ui/mainmenu/screen.go` só mostra a opção "Desfazer uma organização" quando `history.List()` devolve pelo menos um manifesto — não polui o menu de quem nunca organizou nada; erro ao listar é tratado como "sem histórico" só para essa decisão de exibir ou não a opção (o próprio `undo.NewScreen()` reporta o erro de verdade se alcançado por outro caminho).

**`history.List()` devolve `([]Header, []string, error)` — cabeçalhos, avisos, erro — desde a v0.9.0, nunca `[]Manifest` diretamente.** Duas decisões distintas, pelo mesmo motivo de fundo (o "undo" é o recurso que socorre o usuário quando algo deu errado; ele não pode ser, ele mesmo, o ponto único de falha):

- **Um manifesto individual ilegível não interrompe a listagem.** Antes, `List()` chamava `Load(id)` para cada arquivo e devolvia o primeiro erro que encontrasse — um `.yaml` truncado por disco cheio, ou por um processo interrompido no momento errado, abortava a listagem inteira, e com ela o `undo` de **todas** as outras operações, inclusive as íntegras. Agora cada arquivo que falha ao ler ou decodificar gera uma linha em `warnings` (português, citando o nome do arquivo e o motivo) e é pulado; a listagem continua com o resto. `err` só é devolvido quando o **diretório** de histórico em si não pode ser lido (ex: sem permissão) — nunca por causa de um arquivo dentro dele. `undo --list` (`internal/app/undo.go`) e a tela interativa (`internal/ui/undo/screen.go`) imprimem os `warnings` antes da lista; a completação de `undo --id` (`undoIDCompletion`) os DESCARTA — mesma regra transversal já documentada mais abaixo ("nenhuma função de completação propaga erro" vale também para warnings: um Tab não pode cuspir aviso no meio da linha de comando).
- **`Header` (ID, Tool, CreatedAt, InputDir, OutputDir, Action, UndoneAt, EntryCount) substitui `Manifest` completo na listagem** — sem o slice de `Entries`. Antes, listar 200 execuções de 300 arquivos cada retinha 60 mil `Entry` na memória só para imprimir 200 linhas de resumo. `List()` ainda decodifica cada YAML por inteiro (o custo de PARSE continua proporcional ao tamanho total do histórico — não existe hoje um índice separado com só os metadados), mas o `Manifest` decodificado (e o slice `Entries` dentro dele) sai de escopo assim que o `Header` correspondente é montado, dentro do próprio laço — o que fica retido por manifesto na memória depois que `List()` retorna passa a ser O(1), nunca mais O(entradas). `Load(id)` continua devolvendo o `Manifest` completo; é o que `history.Undo` usa de fato. **Evolução futura, não implementada:** se o histórico crescer a ponto do CUSTO DE PARSE (e não mais a memória) incomodar, um índice separado — um arquivo pequeno só com os cabeçalhos, atualizado a cada `Save`/`MarkUndone`/`Prune` — resolveria; hoje não compensa a complexidade extra (mais um arquivo para manter sincronizado, mais um jeito de o histórico ficar inconsistente).

**Exibição limitada a `history.ListDisplayLimit` (20) por padrão** — `undo --list` mostra as 20 mais recentes com um rodapé (`mostrando 20 de 137 — use --all para ver todos`) quando há mais; `--all` remove o limite. A tela interativa (`internal/ui/undo/screen.go`, `selectManifestID`) e o seletor equivalente da linha de comando sem `--id`/`--last` (`internal/app/undo.go`, mesmo nome de função) seguem o mesmo limite, com uma opção extra `"Ver operações mais antigas"` que reapresenta o seletor com a lista completa — um `survey.Select` com centenas de itens é inutilizável, e as duas telas não podiam divergir sobre como lidar com um histórico grande (mesmo princípio de "nunca podem divergir" já usado para `history.Undo`).

**Poda automática — desde a v0.8.1 (só já desfeitas) até a v0.9.0 (também pendentes) — sem ela, o diretório de histórico e `undo --list` cresciam para sempre.** `PruneUndoneAfter` (30 dias, contados de `UndoneAt`) e `PrunePendingAfter` (180 dias, contados de `CreatedAt`) são limiares DELIBERADAMENTE diferentes, com raciocínios diferentes:

- **Já desfeita:** já cumpriu sua função — o único motivo de mantê-la por um tempo é permitir conferir o histórico recente, não desfazer de novo (que exigiria `--force` de qualquer forma). 30 dias é generoso para isso.
- **Pendente (nunca desfeita):** aqui o motivo NÃO é "já cumpriu a função" — é que desfazer algo de 6 meses atrás deixou de ser realista: o destino provavelmente já foi reorganizado ou movido por fora do `file-manager`, e a verificação de tamanho (`SkipSizeChanged`) tornaria o desfazer inútil na prática de qualquer forma. Esta é a poda que faltava na v0.8.1: sem ela, o caso mais comum de uso — organizar e nunca desfazer — continuava acumulando um manifesto por execução, para sempre; só podar os já desfeitos (raro, poucos usuários chegam a desfazer) não resolvia nada para a maioria.

`pruneDetailed(now, undoneAfter, pendingAfter, dryRun)` é a implementação real, não exportada: percorre `List()` (já tolerante a manifesto ilegível) e separa os candidatos em duas categorias — `removedPending`/`removedUndone` — porque `Save()` precisa saber qual categoria perdeu manifestos para decidir se avisa o usuário (só perder um PENDENTE é surpreendente; um já desfeito não). `dryRun` segue o mesmo idioma de `Undo(m, dryRun, force)` já usado neste pacote: `Prune(now, undoneAfter, pendingAfter) ([]string, error)` é a poda de verdade (as duas categorias concatenadas, para quem só quer saber "o que foi removido"); `PrunePlan(now, undoneAfter, pendingAfter) (pending, undone []string, err error)` calcula o mesmo resultado SEM tocar em nada no disco — usado por `undo --prune` para mostrar e confirmar antes de apagar de verdade.

**Regra que não pode ser relaxada, em qualquer um dos dois prazos: um manifesto pendente mais NOVO que `PrunePendingAfter` nunca é removido automaticamente, sob nenhuma condição** — é exatamente ele que permite desfazer aquela operação mais tarde; removê-lo sem o usuário pedir tiraria essa capacidade em silêncio, o oposto do que a feature de desfazer promete.

**`Save()` devolve `(path string, prunedPending []string, err error)`** — o terceiro valor é novo na v0.9.0. A poda em si continua best-effort (erro silenciosamente ignorado — mesmo princípio da Decisão 15/relatório: uma tarefa de manutenção secundária não pode fazer uma gravação que já aconteceu parecer que falhou), mas `prunedPending` NÃO é descartado: apagar um manifesto pendente tira, de verdade, a capacidade de desfazer aquela operação, e isso precisa chegar ao usuário. `historyRecorder` (`internal/tools/organizepdf/command.go`) captura `prunedPending` via um `*[]string` fechado no closure — `pdfutil.OrganizeOptions.Recorder` só devolve `error`, então não há outro jeito de fazer essa informação sair do `Recorder` — e `runWith` acrescenta uma linha em `Result.Details` (`prunedPendingDetail`, ex: `"2 registros de histórico com mais de 180 dias foram removidos e não podem mais ser desfeitos"`) quando não está vazio. Um manifesto já desfeito removido pela mesma poda NÃO entra nessa lista nem gera aviso: já cumpriu sua função, não há capacidade nenhuma sendo tirada em silêncio.

**`undo --prune`** (`internal/app/undo.go`, `runUndoPrune`) poda manualmente, na hora: calcula o plano com `history.PrunePlan`, mostra um resumo (quantos já desfeitos vs. quantos pendentes), pede confirmação — a menos que `-y` — e só então chama `history.Prune` de verdade. `--older-than <dias>` substitui os DOIS prazos padrão pelo mesmo número de dias para os dois; não há hoje um caso de uso que peça limiares diferentes numa poda manual pontual, então um único número mantém a flag simples. `undo` não é uma "ferramenta" do registry (`app.Tools()`), então não é coberto pelo `TestToolsConsistency` de `internal/app/consistency_test.go` — mas isso não significa mais "sem `Doc().Flags` correspondente": desde a correção da lacuna de documentação para IA (ver seção "Exportação de Documentação", abaixo), `undo` tem uma `tool.Doc` própria em `internal/commanddocs`, coberta por um teste equivalente (`TestCommandDocsFlagsMatchCobra`, em `internal/app/command_docs_test.go`).

### 15. Relatório da Organização (`--report`, `internal/pdfutil/report.go`)

O resultado de `organize-pdf` aparecia só resumido na tela (`OrganizeResult.Summary()`), sem nenhum jeito de conferir depois, fora do terminal, por que um arquivo específico foi parar numa pasta ou por que ficou não classificado. Em contexto fiscal (o caso motivador do projeto) isso é rastreabilidade, não conveniência: quem audita um lote de notas fiscais precisa poder abrir uma planilha semanas depois.

**`pdfutil.BuildReport(OrganizeResult) []ReportRow`** é uma função pura (sem I/O) que monta uma linha por arquivo considerado — TANTO `Organized` quanto `Unclassified` — com `Arquivo`, `Origem`, `Destino` (vazio se não classificado), `Classificado` e `Motivo` (traduzido de `Unmatched.Level`/`Pattern` para português, ex: `nível "fornecedor" não encontrado`, `nome do arquivo não encontrado` para `Unmatched.Level == "filename"`).

**Ordenação determinística por `Arquivo`, sempre.** `Organize()` intercala `Organized` e `Unclassified` na ordem em que processa o diretório, que não é estável o bastante para comparar duas execuções do mesmo lote lado a lado (ex: numa planilha, para ver o que mudou entre duas rodadas de calibração). `BuildReport` reordena por nome de arquivo antes de devolver — decisão deliberada, não um acidente de implementação; um relatório cuja ordem varia entre execuções idênticas seria inútil para esse uso.

**CSV com BOM UTF-8 (`WriteReportCSV`), decisão deliberada.** O público desta ferramenta abre o relatório dando duplo-clique no Excel em português. Sem o BOM (3 bytes `EF BB BF` no início do arquivo), o Excel interpreta um CSV UTF-8 como Windows-1252 e qualquer acento sai corrompido — o resto do ecossistema (LibreOffice, Google Sheets, `encoding/csv` lendo de volta) tolera o BOM sem problema, então o custo é só esses três bytes. **Cuidado ao editar `report.go`:** um caractere BOM literal colado direto no código-fonte Go quebra a compilação com "invalid BOM in the middle of the file" — a constante `csvUTF8BOM` precisa ser escrita como escape Unicode (`"﻿"`), nunca como o caractere literal colado no arquivo. A coluna `classificado` sai como `sim`/`nao` (não `true`/`false`): quem lê essa coluna é uma pessoa numa planilha. Grava via `encoding/csv`, nunca concatenando string — caminho de arquivo com vírgula, aspas ou acento é normal aqui (fornecedor com acento, pasta com espaço).

**Falha ao gravar o relatório nunca falha a organização — mesmo princípio já usado para o manifesto de histórico (Decisão 14).** A operação de organizar já aconteceu de verdade quando `writeReportFile` (em `internal/tools/organizepdf/command.go`) é chamado; um erro ali (ex: `--report` aponta para um diretório sem permissão, ou para um diretório já existente) vira uma linha em `OrganizeResult.Warnings` → `tool.Result.Details`, nunca um erro devolvido por `runWith`. Ao contrário do manifesto, a gravação do relatório não é injetada via `Recorder` (que só dispara em execução real): `writeReportFile` é chamado diretamente pelo comando depois de `pdfutil.Organize` retornar, porque **o relatório também precisa ser gerado em `--dry-run`** — é justamente aí que ele mais serve, permitindo conferir a classificação inteira antes de tocar em qualquer arquivo de verdade.

**Validação de `--report-format` acontece ANTES de `pdfutil.Organize` ser chamado**, dentro de `runWith` (`options.go`, `NormalizeReportFormat`). Falhar por um erro de digitação na flag depois de já ter movido ou copiado um lote inteiro seria cruel — o mesmo raciocínio já aplicado a `--filename-regex`/`--level` inválidos.

`Report`/`ReportFormat` (em `Options`) são, ao contrário de `DryRun`/`Sample`, persistidos no perfil salvo (`yaml:"report"`, `yaml:"report_format"`) — é razoável querer sempre gerar o relatório no mesmo caminho toda vez que um perfil calibrado é aplicado.

**Colisão de destino detectada por igual em `--dry-run` e em execução real (`destinationClaimed`, `organize.go`) — corrigido em revisão de PR, não escapar de novo.** Até então, a checagem de colisão só existia dentro do bloco `!opts.DryRun`, e mesmo ali era incidental: um `os.Stat(destAbs)` pensado para pegar colisão com uma execução ANTERIOR também pegava, por acaso, colisão DENTRO do próprio lote — porque o primeiro arquivo já tinha sido fisicamente gravado quando o segundo chegava a verificar o mesmo caminho. Em `--dry-run`, como nada é gravado, esse `os.Stat` nunca via nada: dois arquivos resolvendo para o mesmo destino apareciam os dois como classificados na simulação, e a execução real reclassificava o segundo — exatamente o tipo de divergência que a feature de relatório (`--report`) promete não ter. `destinationClaimed(destAbs, assigned, overwrite)` unifica as duas formas de colisão atrás de uma única checagem, chamada da MESMA forma nos dois modos, ANTES de qualquer gravação:

1. **Colisão dentro do lote:** `assignedDest` (mapa em memória, populado à medida que o loop de `Organize` processa os arquivos) registra o destino de cada arquivo já classificado. Funciona igual em `--dry-run` e execução real, porque não depende de nada estar em disco.
2. **Destino já em disco:** `os.Stat(destAbs)`, sobrevivente de uma execução anterior (ou de fora deste processo). Agora chamado também em `--dry-run`, não só na execução real.

`--overwrite` desliga as duas checagens de uma vez (a intenção de sobrescrever já é explícita — não há colisão a reportar). Um arquivo colidido é reclassificado com `Unmatched{Level: "destino", Pattern: "destino já existe: <caminho>"}` — o `Pattern` já vem pronto para exibição, sem prefixo extra.

**Critério de regressão:** rodar `Organize` duas vezes sobre a MESMA `InputDir` (uma com `DryRun: true`, outra sem), com o MESMO `OutputDir`, precisa produzir `Organized`/`Unclassified` idênticos campo a campo (só o campo `DryRun` do `OrganizeResult` pode diferir) — não só a mesma contagem. Ver `TestOrganizeDryRunMatchesRealRunOnSameBatchCollision` em `internal/pdfutil/organize_test.go`.

**`pdfutil.UnmatchedReason(*Unmatched) string` (report.go) é a fonte única de tradução de `Unmatched` para texto legível em português**, usada tanto na coluna `motivo` do relatório (`BuildReport`) quanto na linha de detalhe de arquivo não classificado que `organize-pdf` imprime na tela (`internal/tools/organizepdf/command.go`, `runWith`). Antes da unificação, a tela tinha sua própria formatação (`nível %q não encontrado` aplicado a QUALQUER `Unmatched.Level`, inclusive a pseudo-etiqueta interna `"destino"`) — o que produzia a mensagem confusa `nível "destino" não encontrado` para uma colisão de destino, dando a entender (incorretamente) que o usuário tinha calibrado mal um nível chamado "destino". `UnmatchedReason` trata `"destino"` como caso especial (devolve `Unmatched.Pattern` direto, sem o formato "nível ... não encontrado"), junto com `"filename"` e `"texto"` — só o `default` (rótulo de nível de fato calibrado pelo usuário) usa o formato `nível %q não encontrado`.

### 16. Hierarquia de Pastas Vinda de uma Planilha (`--csv`, `internal/pdfutil/csvmap.go`)

Até a v0.6.0, a hierarquia de pastas de `organize-pdf` só podia vir de dentro do próprio PDF (`--level`, por regex). O caso motivador desta feature é o inverso: o usuário já tem uma planilha dizendo onde cada documento deve ser arquivado, e o PDF só precisa fornecer a **chave** (ex: número da nota) para procurar naquela planilha. Extrair a hierarquia de dentro do PDF nesse caso seria trabalho perdido e sujeito a erro de regex — a informação já existe, estruturada, na planilha.

**`pdfutil.CSVMap` e `LoadCSVMap(path, keyColumn, levelColumns string/[]string) (CSVMap, error)`** carregam a planilha uma única vez, antes de qualquer PDF ser processado (mesmo princípio de "validar cedo" já usado por `--report-format` e pelos seletores de `organize-pdf` — ver Decisão 11). `CSVMap.Rows` é um `map[string][]string` (chave → componentes de pasta já normalizados); `CSVMap.Order` preserva a ordem de leitura das chaves no arquivo (só existe porque `Rows`, sendo um map, não garante ordem — usado para mostrar "a primeira linha" no resumo interativo). `CSVMap.Warnings` acumula avisos não-fatais (célula de nível vazia).

**Detecção automática de separador, vírgula ou ponto e vírgula.** O Excel em português salva CSV com `;` por padrão, e essa é a planilha que a maioria dos usuários vai ter na mão — mas nada impede uma planilha genuína separada por vírgula. `detectCSVSeparator` interpreta só a PRIMEIRA linha com os dois separadores (via `encoding/csv`, não split ingênuo por string — respeita aspas) e escolhe o que produzir mais colunas; empatar com 1 coluna cada (nenhum dos dois presente) é erro claro, em vez de seguir adiante com uma planilha de coluna única sem sentido.

**BOM UTF-8 descartado no início do arquivo, se presente.** O próprio `file-manager` grava relatórios com BOM desde a v0.6.0 (`WriteReportCSV`, Decisão 15) — é plausível que uma planilha de entrada venha de lá, ou diretamente do Excel, que também grava BOM. Sem descartar, o nome da primeira coluna do cabeçalho viria com esse lixo invisível grudado e nunca casaria com o nome informado em `--csv-key-column`/`--csv-levels`. Reaproveita a constante `csvUTF8BOM` de `report.go` (mesmo pacote) em vez de duplicar o literal.

**Chaves comparadas como texto, com espaços das pontas removidos — NUNCA convertidas para número.** `001` e `1` precisam ser chaves diferentes: zeros à esquerda são significativos em número de nota fiscal. Isso significa que a chave lida da planilha (`strings.TrimSpace`, nunca `strconv.Atoi`) e a chave extraída do PDF via `CSVKeyRegex` (também `TrimSpace`d antes do `Lookup`) precisam concordar exatamente como string — `CSVMap.Lookup` faz o `TrimSpace` do lado da consulta, para que uma chave extraída do PDF com espaço ao redor (regex mal calibrada, ou um PDF com espaçamento estranho) ainda case com a chave da planilha.

**Chave duplicada na planilha é ERRO, não resolvida por sorteio.** Em contexto fiscal, duas linhas apontando para pastas diferentes sob o mesmo número de nota são um problema de preenchimento que só quem fez a planilha pode resolver — indexar "a primeira que aparecer" e seguir em frente esconderia o problema em vez de expô-lo. `LoadCSVMap` falha citando a chave repetida.

**Célula de nível vazia é AVISO, não erro.** Uma planilha real, com centenas de linhas, quase sempre tem alguma célula em branco no meio — impedir o lote inteiro por causa disso destruiria o valor da feature. O componente de pasta correspondente é simplesmente omitido (não vira uma pasta com nome vazio nem `"sem-valor"` nesse caso específico — isso só acontece se o valor SOBREVIVENTE de `NormalizeComponent` ficar vazio depois de sanitizado, ex: célula só com `".."`), e um aviso citando a chave e a coluna chega até `tool.Result.Details` (mesmo padrão de `ocrWarnings` em `command.go`).

**`NormalizeComponent(s string) string`: acentos removidos, espaços viram `_`.** Decompõe com `golang.org/x/text/unicode/norm.NFD`, descarta as marcas de combinação (categoria Unicode `Mn`, via `unicode.Is(unicode.Mn, r)` — é isso que separa "ã" em "a" + til combinante e permite jogar fora só o til), recompõe com `norm.NFC`, troca espaço/tabulação por `_`, colapsa `_` repetidos e por fim passa por `SanitizeFilename` (a mesma sanitização já usada para grupos de captura de `--level`/`--filename-regex`, Decisão 6) — garantindo que `/`, `..` e caracteres inválidos no Windows nunca sobrevivam num nome de pasta vindo da planilha. Resultado vazio vira `"sem-valor"`. **Por quê remover acento**: nome de pasta acentuado é uma fonte real de problema em rede compartilhada (SMB/CIFS com locale mal configurado) e em ambiente misto Windows/Linux (normalização Unicode NFC vs. NFD divergente entre sistemas de arquivo) — não é purismo, é o motivo prático citado pelo dono do projeto.

**`ResolveDestinationCSV` é o par de `ResolveDestination` (Decisão herdada de `organize.go`), não uma reescrita dela.** Aplica `CSVKeyRegex` ao texto do PDF (grupo de captura 1, ou o match inteiro sem grupo — mesma regra de `--level`/`--filename-regex`), resolve a chave via `CSVMap.Lookup`, e monta o caminho com os componentes da planilha + nome do arquivo. **Duas formas de não-classificação, ambas com `Unmatched.Level == "chave"`** (pseudo-etiqueta interna, no mesmo padrão de `"destino"` — `Unmatched.Pattern` já vem pronto como frase final em português, então `UnmatchedReason` devolve `u.Pattern` direto para `"chave"`, sem passar pelo formato genérico `nível %q não encontrado`): a regex não casa com o texto (`"chave não encontrada no documento"`), e a chave encontrada não existe na planilha (`chave "001" não está na planilha` — **este é o caso mais frequente na prática**, por isso a mensagem cita a chave encontrada, para conferir na planilha). Nome de arquivo é a própria chave por padrão; `FilenameRegex`, quando informado, continua valendo e sobrepõe.

**`Organize()` ganhou `OrganizeOptions.CSV *CSVMap` e `CSVKeyRegex *regexp.Regexp`; quando `CSV != nil`, `Levels` é ignorado.** A validação que impede `--csv` e `--level` juntos vive no comando (`ValidateCSVOptions`, `options.go`), não no núcleo — mas `Organize()` se comporta de forma coerente mesmo se alguém chamar o pacote `pdfutil` diretamente com os dois preenchidos (nunca lê `Levels` quando `CSV != nil`), para não depender só da validação de fora. Todo o resto do pipeline (colisão de destino via `destinationClaimed`, `--overwrite`, gravação do manifesto de histórico via `Recorder`, `--report`, `--dry-run`) é o MESMO código do modo `--level` — só a etapa "calcular `dest`/`unmatched` a partir do texto" se bifurca entre `ResolveDestination` e `ResolveDestinationCSV`, exatamente onde a bifurcação já existia entre "com níveis" e "modo somente renomear". **Isso é o que garante, sem esforço extra, que `--dry-run` e execução real continuam idênticos também em modo `--csv`** (mesmo requisito da Decisão 15/Colisão de Destino) — a checagem de colisão nunca soube, e não precisa saber, de onde `dest` veio.

**Fluxo interativo:** a etapa "Calibração dos níveis" virou "Hierarquia de pastas" (mesmo número de etapa, `ui.Step(4, totalConfigSteps, ...)` — `totalConfigSteps` continua 8, nenhum cenário e2e de contagem de passos precisou mudar) e agora começa perguntando, via `survey.Select`, "Pelo conteúdo de cada PDF" ou "Por uma planilha CSV" (`configureLevels`, roteador em `screen.go`). O caminho CSV (`configureCSVHierarchy`) escolhe o arquivo com `filepicker` (extensão `.csv`), mostra um resumo ANTES de qualquer processamento (quantas linhas, coluna-chave, colunas de hierarquia, e um exemplo de caminho gerado a partir da primeira linha — `exampleCSVPath`, usa `CSVMap.Order` para achar essa primeira linha de forma determinística, já que `Rows` é um map), deixa opcionalmente trocar a coluna-chave ou escolher/reordenar as colunas de hierarquia (`askCSVColumns`, lê o cabeçalho completo com `pdfutil.ReadCSVHeader`) e por fim calibra `CSVKeyRegex` **reaproveitando `internal/ui/calibrate`** (rótulo `"chave do documento"`) — o mesmo componente usado para calibrar níveis por conteúdo, sem reimplementar o diálogo de calibração. `recalibrateLevel` ganhou a opção `"Chave do documento (planilha)"`, oferecida só quando `t.opts.CSV != ""`.

### 17. Completação de Shell e Comando `completion` Escondido

O CLI tem dois públicos muito diferentes: o usuário final, não técnico, que abre o `.exe` com duplo clique no Windows e usa o menu interativo; e o dono do projeto, que usa terminal (zsh) no Linux. O cobra acrescenta sozinho um subcomando `completion`, que gera o script de completação de shell — útil só para o segundo público, e a única peça deste CLI em inglês (junto de `help` e do texto de `--help`), num programa todo em português.

**Escondido, nunca desativado.** `root.CompletionOptions.HiddenDefaultCmd = true` (em `internal/app/root.go`) tira `completion` da lista de comandos mostrada por `--help`, mas o comando continua existindo e funcionando de verdade para quem o invoca diretamente (`file-manager completion zsh`). `DisableDefaultCmd` (que removeria a funcionalidade) **não** é usado de propósito: prejudicaria quem usa terminal sem beneficiar em nada quem usa o menu interativo — o problema nunca foi o comando existir, foi ele aparecer em destaque para quem nunca vai precisar dele.

**Textos traduzidos onde o cobra permite, e só onde permite.** O comando `help` (Short/Long em inglês por padrão) foi substituído via `root.SetHelpCommand(newHelpCommand(root))` — ver `internal/app/help.go`, uma cópia adaptada do comando interno do cobra (`Command.InitDefaultHelpCmd`, não exportado), sem a propagação de `cmd.ctx` (campo não exportado, e desnecessário aqui: `--help` nunca executa `RunE`, só imprime texto). O texto de `--help` ("help for `<comando>`") foi traduzido registrando uma flag **persistente** `help` no comando raiz (`root.PersistentFlags().BoolP("help", "h", false, "ajuda sobre este comando")`) em vez de deixar o cobra criar, sob demanda, uma flag local por comando via `InitDefaultHelpFlag` — como cada subcomando herda as flags persistentes de seus ancestrais antes de `InitDefaultHelpFlag` rodar, o cobra encontra `help` já registrada e não sobrescreve com a versão em inglês. Efeito colateral cosmético e aceito: `--help` passa a aparecer em "Opções globais" (ver template abaixo) em vez de "Opções" no help de cada subcomando, e o texto é o mesmo genérico para todo comando (perdendo o nome específico que o cobra incluiria dinamicamente). **O texto do próprio comando `completion` (Short/Long dos subcomandos `completion bash`/`completion zsh`/etc.) permanece em inglês** — não é razoavelmente configurável sem reescrever à mão a lógica interna de geração desses subcomandos (`Command.InitDefaultCompletionCmd`, não exportada), e como o comando já está escondido de `--help`, o custo de deixá-lo em inglês é baixo.

**Rótulos estruturais da ajuda traduzidos via `SetUsageTemplate` — reavaliação deliberada de uma decisão anterior.** A primeira versão desta feature deixou de propósito o `UsageTemplate` do cobra intocado ("Usage:", "Available Commands:", "Flags:" continuavam em inglês), por cautela: um template reescrito à mão é frágil e pode quebrar silenciosamente numa atualização da biblioteca. Revisão de PR encontrou o resultado — descrições em português, rótulos estruturais em inglês — pior do que não traduzir nada: parece trabalho inacabado, e é a saída mais vista do programa. `SetUsageTemplate`/`SetHelpTemplate` são API pública e documentada do cobra, feita exatamente para isso; o risco de divergência numa atualização futura é real mas de degradação branda (uma seção nova da biblioteca deixa de ser traduzida, nada quebra) — vale o custo. `usageTemplatePT` (`internal/app/usage.go`) é uma **cópia adaptada** de `defaultUsageTemplate` (cobra v1.10.2, `command.go`): só o texto literal dos rótulos foi trocado (`Usage`→`Uso`, `Aliases`→`Apelidos`, `Examples`→`Exemplos`, `Available Commands`→`Comandos disponíveis`, `Additional Commands`→`Comandos adicionais`, `Flags`→`Opções`, `Global Flags`→`Opções globais`, `Additional help topics`→`Tópicos de ajuda adicionais`, o rodapé `Use "..." for more information about a command.`→`Use "..." para mais informações sobre um comando.` mantendo `{{.CommandPath}}`, e o placeholder `[command]`→`[comando]` nos dois lugares em que aparece) — toda a lógica de template (`{{if}}`/`{{range}}`, os guardas `HasAvailableSubCommands`/`HasAvailableLocalFlags`/`HasExample`/etc., `rpad`, `trimTrailingWhitespaces`) foi preservada byte a byte, na mesma ordem. Aplicado só em `root.SetUsageTemplate(usageTemplatePT)`, dentro de `NewRootCommand`: o cobra propaga o template do pai para todo comando filho que não registra o seu próprio (`Command.UsageTemplate()` sobe a árvore recursivamente até achar um definido), então cada subcomando herda automaticamente — confirmado na prática (`TestSubcommandHelpInheritsPortugueseTemplate`, `internal/app/usage_test.go`), não presumido. `defaultHelpTemplate` do cobra não contém nenhum rótulo fixo em inglês (só encaixa `.Long`/`.Short`, já em português, dentro de `.UsageString`) — por isso `SetHelpTemplate` não foi necessário.

**Duas peças em inglês continuam fora de alcance, mesmo depois do `SetUsageTemplate`, e não há gambiarra razoável para nenhuma das duas — não insista nelas sem repensar a abordagem inteira:**

1. **O `[flags]` na linha "Uso:" (ex: `file-manager merge-pdf [flags]`) vem de `Command.UseLine()`, um método Go, não do template.** `UseLine()` concatena `" [flags]"` como string literal hardcoded na própria biblioteca (`command.go`, sem nenhum hook de customização) sempre que o comando tem flags disponíveis; o template só chama `{{.UseLine}}`, sem controle sobre o que esse método devolve. Corrigir isso exigiria ou sobrescrever `Use` de cada comando manualmente incluindo a string exata `"[flags]"` (o `UseLine` só pula a concatenação se a string já contiver esse literal em inglês — não haveria como fazer aparecer `"[opções]"` por esse caminho) ou copiar `UseLine()` inteiro para fora do pacote, o que está fora de proporção para um único token.
2. **Mensagens de erro do próprio cobra (ex: `unknown command "x" for "file-manager"`, erros de parsing de flag) continuam em inglês.** Não são geradas pelo `UsageTemplate`/`HelpTemplate`; vêm de `Command.execute()`/`ValidateArgs`/pflag, sem ponto de customização exposto além de interceptar `SilenceErrors` e reformatar o erro por conta própria (o que exigiria reconhecer e traduzir cada formato de erro do cobra por string matching — frágil, e quebra silenciosamente a cada atualização da dependência). Fora de escopo desta feature; registrado aqui para quem vier depois não gastar tempo procurando uma forma de configurar isso que não existe.

**Completação de valores, não só de nomes.** Nomes de comando e flag o cobra já completa de graça; o ganho real para quem usa terminal é completar **valores**, via `cmd.RegisterFlagCompletionFunc`. Duas categorias:

- **Enums fixos** (`cobra.FixedCompletions`, sem I/O): `split-pdf --mode` (`page`/`range`/`regex`), `--ocr` em `split-pdf`/`organize-pdf` (`auto`/`always`/`never`), `organize-pdf --report-format` (`csv`/`json`), `merge-pdf --sort` (`name`/`mtime`).
- **Valores dinâmicos** (I/O real): `undo --id` lê `history.List()` e oferece só manifestos com `UndoneAt == nil` — sugerir um ID já desfeito levaria a um erro evitável ("já foi desfeita"); `profiles list --tool` e `profiles export --tool` (`profileToolCompletion`, em `internal/app/root.go`) leem `Tools()` filtradas por `Profile() != nil`; `profiles import --file` e `organize-pdf --csv` delegam a completação de arquivo ao cobra via `cobra.ShellCompDirectiveFilterFileExt` (extensões `yaml`/`yml` e `csv`, respectivamente), em vez de listar candidatos manualmente. O formato `"<valor>\t<descrição>"` (texto depois do TAB) é o que o zsh mostra como descrição ao lado de cada opção; o bash ignora essa parte.
- **`organize-pdf --csv-levels`** (`csvLevelsCompletion`, em `internal/tools/organizepdf/command.go`) lê o cabeçalho da planilha que o usuário já digitou em `--csv` — via `cmd.Flags().GetString("csv")`, o valor já presente no `FlagSet` do próprio comando em execução, não uma segunda leitura de `os.Args` — e devolve os nomes de coluna via `pdfutil.ReadCSVHeader` (mesma função usada pelo fluxo interativo para o mesmo propósito, Decisão 16). Sem `--csv` preenchido, ou com um caminho que não existe/não pode ser lido (planilha ainda não criada, digitação pela metade), devolve lista vazia sem erro — o caso mais comum na prática, já que o usuário normalmente está completando `--csv-levels` logo depois de `--csv`, muitas vezes antes de terminar de digitar o caminho da planilha.
- **`--ocr-lang`** (`ocr.CompletionLanguages()`, em `internal/ocr/tesseract.go`) tenta listar os idiomas de fato instalados via `tesseract --list-langs`, mas com um limite de tempo curto (`completionLanguageTimeout`, 300ms, via goroutine + `select`/`time.After`): sem o tesseract disponível, ou se ele não responder a tempo, devolve a lista fixa conhecida (`por`, `eng`) em vez de travar a tecla Tab do usuário esperando um processo externo.

**Regra transversal, sem exceção: nenhuma função de completação propaga erro.** `config.List`, `history.List` e qualquer outra fonte de dados dentro de uma função de completação que devolva erro resulta em lista vazia + `cobra.ShellCompDirectiveNoFileComp`, nunca em erro repassado ou mensagem impressa. Um Tab que cospe erro no meio da linha de comando é pior, para quem está digitando, do que um Tab que simplesmente não completa nada — e completação roda toda vez que o usuário aperta Tab, então qualquer lentidão ou instabilidade ali é sentida imediatamente, ao contrário de um erro no corpo de um comando (que só aparece quando o usuário decide rodar de verdade).

**`--profile` não existe hoje.** Um perfil salvo só é aplicado hoje pela tela interativa (`internal/ui/profiles/`); nenhum comando cobra tem uma flag `--profile` para aplicar um perfil diretamente por linha de comando. Se essa flag for adicionada no futuro, ela deveria ganhar completação dinâmica a partir de `config.List(<toolID>)`, seguindo o mesmo padrão de `profileToolCompletion`.

### 18. PDF Pesquisável a partir de Digitalização (`ocr-pdf`, `internal/pdfutil/ocrize.go`)

Até a v0.10.x, o OCR do CLI (Decisão 7) só servia para **leitura**: o texto reconhecido ficava em memória, usado uma única vez para casar uma regex, e o arquivo continuava sendo imagem — não pesquisável no Explorer do Windows nem em leitor de PDF, e cada execução reprocessava tudo do zero. `ocr-pdf` fecha essa lacuna: grava a camada de texto de volta no arquivo, gerando um PDF novo (nunca sobrescrevendo o original).

**Viabilidade, medida antes de implementar (não reabrir essa investigação):** `tesseract <imagem> <saida> -l por pdf` gera um PDF de uma página com a imagem original e uma camada de texto invisível sobreposta — verificado processando o resultado com o próprio CLI em `--ocr never` e confirmando que a regex casou (o mesmo teste sobre o PDF escaneado original não casa nada, prova de que o texto está mesmo embutido, não é coincidência). Custo: ~0,9s por página. Custo de tamanho: um arquivo de 128 KB virou 241 KB — o Tesseract reescreve a imagem ao montar o PDF pesquisável, então o resultado é sempre maior que o original.

**A limitação que define o desenho, e por quê a regra é conservadora.** A abordagem inteira reconstrói o PDF de saída a partir das imagens extraídas de cada página (a mesma extração que o fallback de OCR de leitura já usa — Decisão 7). Numa página puro-scan (uma única imagem, sem texto embutido), isso é fiel ao original. Numa página **mista** — imagem + texto nativo, vetores, ou mais de uma imagem — o conteúdo que não é aquela única imagem **seria descartado** silenciosamente. Por isso `DecideEligibility` recusa qualquer arquivo que tenha uma página fora do padrão "toda página é puro scan", em vez de processar parcialmente: destruir conteúdo em silêncio é sempre pior que recusar o arquivo com um motivo explícito. As decisões de eligibilidade, cada uma com seu motivo específico (ver `ocrize.go`, `DecideEligibility`):

- Toda página `PagePureScan` (exatamente 1 imagem, sem texto) → elegível.
- Qualquer página `PageMixed` (imagem + texto, ou ≥2 imagens) → recusado: reconstruir perderia esse conteúdo extra.
- **Todas** as páginas já com texto (`PageHasText`) → recusado por **economia**, não por erro: o arquivo já é pesquisável, não há o que reconhecer.
- Mistura de `PagePureScan` e `PageHasText` → recusado: só parte do arquivo é digitalizada, reconstruir perderia o texto das páginas restantes.
- Alguma `PageNoImage` (nem imagem, nem texto) junto de páginas de scan → recusado, mesmo raciocínio.
- Zero páginas → recusado.

`ClassifyPages`/`DecideEligibility` são funções **puras** (sobre `[]string` de texto por página + `map[int]int` de contagem de imagens por página, sem tocar em PDF nenhum) — testáveis exaustivamente sem depender de tesseract nem de fixture de PDF real.

**Motor de OCR→PDF declarado em `pdfutil`, não importado de `internal/ocr` — mesmo padrão de `OCREngine` (Decisão 7/textextract.go).** `SearchablePDFEngine` (`Available() bool`; `ImageToSearchablePDF(ctx, imagePath, outBase, lang) error`) é uma interface local a `ocrize.go`; `internal/ocr.Tesseract` ganhou o método `ImageToSearchablePDF` (roda `tesseract <img> <outBase> -l <lang> pdf`, produz `<outBase>.pdf`) que a satisfaz por duck typing, sem `pdfutil` precisar importar `internal/ocr`. Isso mantém o núcleo testável com um motor falso (`fakeSearchablePDFEngine` em `ocrize_test.go`, que grava um PDF mínimo real via a mesma fixture `buildTestPDF` dos testes de integração do pacote, em vez de rodar tesseract de verdade) — sem processo externo, sem rede, determinístico.

**`OCRize` exige o motor disponível ANTES de processar qualquer coisa — inclusive em `--dry-run`.** Ao contrário do OCR de leitura (opcional, degrada silenciosamente sem o Tesseract), aqui o motor é o próprio propósito da ferramenta: simular sem o Tesseract instalado prometeria uma execução real que não vai funcionar. `internal/tools/ocrpdf` falha com `ocr.InstallHint()` antes de resolver qualquer entrada.

**Ordenação por NÚMERO DE PÁGINA, nunca por nome de arquivo — armadilha real, testada explicitamente.** O nome do arquivo de imagem extraída pelo pdfcpu (`<base>_<página>_<prefixo><índice>.<ext>`) não ordena alfabeticamente na mesma sequência da numeração: para 10 páginas, `"..._1_..."` vem antes de `"..._10_..."`, que vem antes de `"..._2_..."`. `ocrizeOneFile` evita esse problema pela raiz: gera o PDF de cada página com um nome próprio, **zero-padded por número de página** (`pagina-%05d.pdf`), então mesmo a ordenação alfabética que `pdfutil.Merge`/`ResolveInputs` aplica por padrão (`Sort: "name"`) já corresponde à ordem numérica correta — não depende de acertar a ordenação de `Merge` para o caso geral, resolve no nome do arquivo temporário que a própria função controla. `TestOCRizePreservesPageOrderWithDoubleDigitPages` (`internal/pdfutil/ocrize_test.go`) constrói um PDF real de 10 páginas puro-scan e confere, lendo o texto embutido do arquivo final, que a página N contém o marcador da página N — é exatamente o caso (10 páginas) em que a ordenação textual ingênua erraria.

**Nunca sobrescreve o original.** Destino = `<OutputDir ou pasta do original>/<nomeBase><Suffix>.pdf`; se o caminho calculado colidir com o próprio arquivo de origem (ex: `--suffix ""` sem `--output-dir`), a entrada é recusada com um motivo em vez de gravar por cima. Destino já existente: `--skip-existing` pula sem erro (pensado para retomar um lote grande interrompido — a ~0,9s/página, 200 documentos de 3 páginas levam quase 10 minutos), e sem `--overwrite` também pula — nunca falha o lote inteiro por causa de um arquivo.

**Falha num arquivo nunca derruba o lote; cancelamento de contexto, sim.** `ocrizeOneFile` só devolve erro de verdade quando `ctx.Err()` dispara (checado entre arquivos E entre páginas, dentro do laço de geração por página — um lote grande precisa ser interrompível com Ctrl+C sem deixar lixo, já que cada `os.MkdirTemp` de arquivo tem `defer os.RemoveAll`). Qualquer outra falha (extração de imagem, OCR de uma página, `Merge` final) vira uma entrada em `Skipped` com o motivo, e `OCRize` segue para o próximo arquivo.

**Relatório (`--report`) reaproveita o padrão da v0.6.0 (Decisão 15), com colunas próprias.** `BuildOCRizeReport`/`WriteOCRizeReportCSV` (`internal/pdfutil/ocrize.go`/`report.go`) seguem exatamente o mesmo BOM UTF-8 (`csvUTF8BOM`, reaproveitada, não duplicada) e ordenação determinística por nome de arquivo do relatório de `organize-pdf`, mas com colunas adequadas a esta ferramenta (`arquivo, origem, destino, processado, paginas, motivo` — `processado` em vez de `classificado`, `sim`/`nao`).

**Progresso impresso arquivo a arquivo, indispensável no custo desta ferramenta.** `OCRizeOptions.Progress func(done, total int, path string)` é chamado uma vez por arquivo processado (elegível ou não). `internal/tools/ocrpdf` monta a linha (`formatProgressLine`, `command.go`) no formato `[3/120] nota-003.pdf — 2 página(s)...` — função pura, usada tanto pelo comando cobra (`fmt.Println`) quanto pela tela interativa (`ui.Infof`), para as duas nunca divergirem na redação. Sem isso, um lote de 200 documentos de 3 páginas (quase 10 minutos) pareceria travado.

**Tela interativa sempre simula antes de aplicar — mesmo espírito do ciclo de teste de `organize-pdf` (Decisão 4/Dry-run compartilhado), sem a complexidade de calibração de regex que `organize-pdf` tem.** `internal/tools/ocrpdf/screen.go` roda `ocrizeRaw(dryRun=true, ...)` incondicionalmente, mostra quantos arquivos são elegíveis e quantos seriam pulados (com o motivo), e só então pergunta confirmação antes de rodar de verdade com `dryRun=false` — nunca toca em um arquivo sem que o usuário tenha visto o resultado da simulação primeiro. `ocrizeRaw` devolve tanto o `pdfutil.OCRizeResult` "cru" (para a tela decidir se há algo elegível a aplicar) quanto o `tool.Result` já formatado (usado pelo comando cobra, que não precisa inspecionar a contagem) — `tool.Result` só tem `Summary`/`Details` (strings), insuficiente para essa decisão.

### 19. Seleção Vazia Nunca é Aceita em Silêncio (`filepicker.PickFiles`, `ocr-pdf`, `merge-pdf`)

**O defeito relatado.** Um usuário usou `ocr-pdf` pelo menu, navegou até uma pasta que **tinha** um PDF, escolheu "Escolher arquivos específicos" e, na tela de `survey.MultiSelect`, apertou Enter sem antes apertar a barra de espaço (em `survey.MultiSelect`, navegar é com as setas, **marcar é com espaço**; Enter só confirma o que já estiver marcado — ver Decisão 13). O programa aceitou a seleção vazia em silêncio e seguiu por **mais seis perguntas** (sufixo, idioma, sobrescrita, retomada, e a simulação em si) antes de falhar com `"informe ao menos um arquivo ou pasta em --input"`. Dois defeitos somados: a dica que resolveria a dúvida estava em inglês (corrigido na Decisão 13) e a seleção vazia não era barrada no ato.

**Regra, sem ambiguidade: com zero entradas marcadas, o fluxo NUNCA avança para as perguntas seguintes.** Os dois desfechos aceitáveis são (a) repetir a própria etapa de seleção, avisando na hora e lembrando da barra de espaço — o desfecho preferido, porque obrigar a refazer toda a navegação de pastas seria punir o usuário por um erro da interface, não dele — ou (b) encerrar o fluxo (voltar ao menu), se o usuário optar por isso ou se o limite de tentativas estourar. Avisar e seguir mesmo assim, ou só validar no final, não é aceitável — foi exatamente o defeito relatado.

**Este é o mesmo padrão que `organize-pdf` já corrigira na v0.2.1 (Decisão 11, "pasta de origem vazia só era percebida no final") — e que a ferramenta nova (`ocr-pdf`, v0.11.0) não herdou.** `organize-pdf` e `ocr-pdf`/`merge-pdf` resolvem um problema parecido em pontos diferentes do fluxo porque a forma de escolher entradas é diferente: `organize-pdf` pede uma única pasta (`pickInputDir`, valida contando PDFs com `countPDFs`); `ocr-pdf`/`merge-pdf` deixam misturar arquivos avulsos e pastas inteiras num laço (`collectInputsOnce`) que usa `filepicker.PickFiles` para a marcação múltipla. A correção acompanha essa diferença, em duas camadas:

1. **Na origem, em `internal/ui/filepicker/filepicker.go`:** `pickMarkedFiles` (chamada por `PickFiles` ao entrar em `[ Escolher arquivos desta pasta ]`) pergunta via `survey.MultiSelect` e, se a resposta vier com zero itens marcados, avisa (mencionando explicitamente a barra de espaço) e pergunta se quer tentar de novo na mesma pasta — até `maxEmptySelectionAttempts` (3) vezes; a essa altura, ou se o usuário recusar, devolve `ErrCancelled`. `PickFiles` **nunca** devolve `([]string{}, nil)` — só um slice não-vazio ou um erro. Separadamente, uma pasta sem nenhum arquivo da extensão pedida (`len(files) == 0`) nem chega a mostrar a lista (que estaria vazia e não comunicaria nada): `emptyDirMessage(dir, exts)` avisa qual pasta e qual extensão, e volta à navegação para escolher outra. As duas decisões (mensagem + desistir ou não) são funções puras (`emptySelectionAdvice`, `emptyDirMessage`), testadas sem terminal.
2. **No agregado, em `internal/tools/ocrpdf/command.go` e `internal/tools/mergepdf/command.go`:** `pickInputs` (o `Prompt` do parâmetro `"input"`, chamado por `tool.PromptAll` antes de qualquer outro parâmetro ser perguntado) só devolve com sucesso quando `t.opts.Inputs` tem pelo menos um item; com zero, avisa (`emptyInputsAdvice`, mesma forma pura acima) e oferece tentar de novo ou desistir, também limitado a 3 tentativas. Na prática esta camada dificilmente é acionada — a correção em `PickFiles` já impede o caso mais comum na raiz —, mas fica como última linha de defesa: o defeito original era justamente uma contagem zero atravessando sem ninguém perceber, e `pickInputs` é o que garante, de forma explícita e testável, que o restante de `PromptAll` (sufixo, idioma, sobrescrita, retomada em `ocr-pdf`; caminho de saída em `merge-pdf`) nunca é perguntado com `Inputs` vazio.

**`ocr-pdf` e `merge-pdf` duplicam o bloco inteiro de "como adicionar entradas"** (mesmo texto de pergunta, mesma lógica de profundidade de pasta) — duplicação pré-existente ao lançamento de `ocr-pdf`, não introduzida por esta correção. A correção foi aplicada **identicamente** nas duas cópias, de propósito, para não deixar o defeito vivo numa das duas — mas a duplicação em si permanece: um refactor futuro que extraia esse bloco para um helper compartilhado (ex: em `internal/ui/filepicker` ou um novo pacote) eliminaria o risco de as duas cópias divergirem de novo, como quase aconteceu aqui.

### 20. Flag `--version`/`-v` Convive com o Subcomando `version` (Saída Idêntica)

Até a v0.11.0, a única forma de consultar a versão era `file-manager version`; `--version`/`-v` — convenção quase universal em CLIs — devolvia "flag desconhecida" e código de saída 1, a mesma impressão de "programa mal-acabado" já discutida na Decisão 17 para partes em inglês.

**As duas formas convivem, de propósito — nenhuma substitui a outra.** Remover o subcomando quebraria quem já o usa (inclusive `internal/selfupdate.VerifyBinary`, que roda `<binário-baixado> version` para validar um download antes de substituir o executável em uso — ver Decisão 12); não acrescentar a flag deixaria o reflexo mais comum de todo usuário de CLI sem resposta.

**As duas formas produzem a MESMA saída, byte a byte, em stdout — este é o ponto que exigiu atenção, não a flag em si.** O template padrão do cobra (`defaultVersionTemplate`) imprimiria algo como `file-manager version v0.11.0 (...)`, diferente do que `file-manager version` sempre imprimiu (`v0.11.0 (...)`, sem o nome do binário nem a palavra "version" na frente). Ter duas saídas diferentes para a mesma informação é exatamente o tipo de divergência que este projeto evita em outros lugares (dry-run vs. execução real — Decisão 4; tela vs. comando de `undo` — Decisão 14): um script feito em cima de uma das duas formas seria surpreendido pela outra. `TestVersionFlagMatchesVersionSubcommand` (`internal/app/version_flag_test.go`) e `TestVersionFlagMatchesSubcommand` (`e2e/version_flag_test.go`) travam essa igualdade nos dois níveis (chamada direta ao `cobra.Command` e binário real).

**A flag é registrada manualmente, INDEPENDENTE do mecanismo embutido do cobra.** `Command.InitDefaultVersionFlag` (chamado automaticamente e de forma idempotente por `Execute()`, inclusive ao montar `--help`) só cria a flag `--version` quando `c.Flags().Lookup("version") == nil` — gerando, quando cria, uma descrição fixa em inglês (`"version for file-manager"`), sem gancho de tradução. `root.Flags().BoolP("version", "v", false, "mostra a versão do binário")`, registrado dentro de `NewRootCommand` antes de qualquer execução, faz o cobra pular a criação da dele. O atalho `-v` foi confirmado livre (nenhuma ferramenta nem subcomando deste CLI o usa) antes de reivindicá-lo explicitamente, em vez de depender do fallback silencioso do cobra (que só usa `-v` quando `ShorthandLookup("v") == nil` de qualquer forma).

**Mecanismo mudou na v1.0.0: `root.Version` ficou vazio, e a flag é tratada dentro do `RunE`, não pelo atalho embutido do cobra.** Até a v0.12.0, `root.Version = v.String()` era o gatilho que o cobra usa (em `Command.execute`) para tratar `--version` como saída antecipada ANTES de qualquer `RunE` rodar — o mesmo mecanismo que intercepta `--help` —, e `root.SetVersionTemplate("{{.Version}}\n")` fechava a lacuna do template. Isso bastava enquanto `--version` só precisava imprimir uma linha, mas deixou de servir quando a v1.0.0 acrescentou o aviso de atualização disponível (ver a entrada correspondente na Decisão 12, acima): esse aviso só pode ser impresso DEPOIS da linha de versão, e o atalho embutido do cobra retorna assim que imprime, sem dar nenhum gancho para código rodar em seguida. Por isso `root.Version`/`root.SetVersionTemplate` foram removidos e o tratamento de `--version`/`-v` passou para dentro do `RunE` do comando raiz (lendo a flag manualmente via `cmd.Flags().GetBool("version")`), chamando a mesma função `printVersion` que `newVersionCommand` usa — o que torna a igualdade de saída entre as três formas estrutural (mesma função chamada pelas três) em vez de mantida por um template do cobra. Ver a entrada correspondente na Decisão 12 para o raciocínio completo do aviso em si (por que `IsOutputTerminal()` e não `IsInteractive()`, por que stderr, por que o timeout é curto).

**Se um refactor futuro fizer o `RunE` da flag `--version` parar de chamar `printVersion` (ex: voltar a montar a linha de versão "na mão" ali dentro), a saída de `--version` volta a poder divergir da de `version` silenciosamente** — os testes citados acima existem para pegar exatamente isso.

### 21. Congelamento da Superfície Pública a partir da v1.0.0

A v1.0.0 é a declaração de estabilidade do projeto (ver também `CHANGELOG.md`, seção "Política de Versionamento"): a partir dela, duas superfícies ficam **congeladas** e só podem mudar de forma incompatível numa `2.0.0`, seguindo a regra de MAJOR que já estava escrita antes mesmo de existir uma 1.0 para aplicá-la:

- **O formato do YAML dos perfis salvos** (Decisão 5) — inclusive os nomes dos campos do envelope (`name`, `tool`, `created_at`, `updated_at`, `data`) e da estrutura interna de `data` de cada ferramenta. Importa em dobro porque perfis são pensados para serem **exportados e trocados entre máquinas** (`profiles export`/`import`, Decisão 5): um perfil calibrado numa máquina com uma versão precisa continuar importável numa máquina com outra versão 1.x.
- **Os nomes e a semântica das flags já existentes** de cada ferramenta e subcomando — renomear, remover ou mudar o que uma flag faz quebra automação de quem já usa o `file-manager` em script.

**O que NÃO está congelado:** acrescentar uma ferramenta nova, um subcomando novo, uma flag nova, ou um campo novo e opcional numa struct de opções de perfil (que não quebra a leitura de um perfil salvo por uma versão anterior) — isso é o que MINOR continua cobrindo normalmente, dentro da série 1.x.

Esta decisão existe para que uma mudança futura numa dessas duas superfícies não passe despercebida numa revisão de código — quem for renomear uma flag existente, ou mudar um campo do YAML de perfil, precisa parar e considerar se isso não deveria ser uma `2.0.0`, não uma continuação normal da série 1.x.

## Fluxo para Adicionar Uma Ferramenta Nova

### Passo 1: Gerar esqueleto

```bash
make new-tool NAME=minha-ferramenta
```

Cria:
```
internal/tools/minha-ferramenta/
  ├── command.go         (esqueleto; implementar params(), Execute())
  ├── options.go         (struct de opções)
  ├── screen.go          (esqueleto; implementar ExecuteInteractive())
  ├── tool.go            (esqueleto; implementar Doc(), Profile())
  └── minha_ferramenta_test.go
```

### Passo 2: Implementar lógica pura

Cria um pacote em `internal/<dominio>util/` (ex: `internal/mytoolutl/`):

```
internal/mytoolutl/
  ├── mytoolutl.go        (funções puras: MyOperation(...))
  └── mytoolutl_test.go   (testes de tabela)
```

Nenhuma dependência de tela, arquivo ou I/O — 100% testável.

### Passo 3: Preencher `internal/tools/minha-ferramenta/`

#### `options.go`
Define struct que armazena as opções:

```go
type Options struct {
    Input    string
    Output   string
    MyParam  bool
}
```

#### `command.go` — função `params()`
Declare cada parâmetro uma única vez:

```go
func (t *Tool) params() []tool.Param {
    return []tool.Param{
        {
            Name:        "input",
            Shorthand:   "i",
            Type:        "string",
            Description: "Arquivo de entrada",
            BindFlag: func(fs *pflag.FlagSet) {
                fs.StringVarP(&t.opts.Input, "input", "i", "", "...")
            },
            Prompt: func() error {
                return survey.AskOne(&survey.Input{...}, &t.opts.Input)
            },
        },
        // ... outros params
    }
}
```

#### `command.go` — função `Execute()`
Orquestração: validar, chamar lógica pura, retornar resultado:

```go
func (t *Tool) Execute(opts Options) (tool.Result, error) {
    // Validar
    // Chamar internal/mytoolutl.MyOperation(opts)
    // Retornar tool.Result
}
```

#### `screen.go` — `ExecuteInteractive()`
Só I/O: perguntar, validar, chamar `Execute()`:

```go
func (t *Tool) ExecuteInteractive(ctx context.Context, nav *ui.Navigator) error {
    if err := tool.PromptAll(t.params()); err != nil {
        return err
    }
    // Validar t.opts
    result, err := t.Execute(t.opts)
    // Mostrar resultado na tela
    return nil
}
```

**Não teste linha a linha.** É deliberadamente não testado.

#### `tool.go`
Implementar contrato `tool.Tool`:

```go
func (t *Tool) Meta() tool.Meta {
    return tool.Meta{
        ID:          "minha-ferramenta",
        Title:       "Minha Ferramenta",
        Description: "Uma breve descrição",
    }
}

func (t *Tool) Command() *cobra.Command {
    cmd := &cobra.Command{...}
    tool.BindAll(cmd.Flags(), t.params())
    cmd.RunE = func(cmd *cobra.Command, args []string) error {
        result, err := t.Execute(t.opts)
        // ... mostrar resultado
        return err
    }
    return cmd
}

func (t *Tool) Screen() ui.Screen {
    return NewScreen(t)
}

func (t *Tool) Doc() tool.Doc {
    return tool.Doc{
        Name:        "minha-ferramenta",
        Title:       "Minha Ferramenta",
        Description: "Uma breve descrição",
        // DocFlags preenchido automaticamente de params()
    }
}

func (t *Tool) Profile() tool.ProfileSupport {
    return &profileSupport{tool: t}
}
```

### Passo 4: Registrar em `internal/app/registry.go`

```go
func Tools() []tool.Tool {
    return []tool.Tool{
        mergepdf.New(),
        splitpdf.New(),
        organizepdf.New(),
        minhaiferramenta.New(),  // ← adicionar aqui
    }
}
```

### Passo 5: Escrever testes

Testes de tabela para lógica pura (`internal/mytoolutl/mytoolutl_test.go`):

```go
func TestMyOperation(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {
            name:    "success case",
            input:   "example",
            want:    "result",
            wantErr: false,
        },
        {
            name:    "error case",
            input:   "invalid",
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := MyOperation(tt.input)
            if (err != nil) != tt.wantErr {
                t.Fatalf("got error %v, wantErr %v", err, tt.wantErr)
            }
            if !tt.wantErr && result != tt.want {
                t.Errorf("got %q, want %q", result, tt.want)
            }
        })
    }
}
```

Evite testes frágeis de integração ou mocks complexos.

### Passo 6: Compilar, testar, verificar

```bash
make test   # Deve passar
make lint   # Deve passar (ou rodar make fmt)
make build  # Deve compilar
```

### Passo 7: Atualizar documentação

1. **README.md:** Adicione uma seção com tabela de flags e 2 exemplos reais
2. **CHANGELOG.md:** Adicione entrada em `[Não publicado]`

## Comandos de Build, Teste e Utilidade

Todos via `make`:

| Comando | O que faz |
|---------|-----------|
| `make build` | Compilar para a plataforma atual em `dist/file-manager` |
| `make build-linux` | Compilar para Linux amd64 |
| `make build-windows` | Compilar para Windows amd64 |
| `make build-all` | Compilar para Linux e Windows |
| `make test` | Rodar testes com coverage e race detector |
| `make e2e` | Rodar os testes ponta a ponta (terminal virtual, só Linux, mais lentos) — ver seção própria abaixo |
| `make lint` | Executar `go vet` e `golangci-lint` (se instalado) |
| `make fmt` | Formatar código com gofmt |
| `make new-tool NAME=x` | Gerar esqueleto de ferramenta nova |
| `make docs` | Exportar documentação em `dist/` (formatos context e skill) |
| `make clean` | Remover artefatos de build (`dist/`) |

**Build com flags:** O Makefile injeta versão, commit e data via `-ldflags` (variáveis `main.version`, `main.commit`, `main.date`).

**CGO:** Todas as compilações usam `CGO_ENABLED=0` (Go puro, sem C).

## Testes Ponta a Ponta (E2E)

**Por que existem:** o projeto tem 16 pacotes de teste verdes com `-race` e, ainda assim, três defeitos sérios chegaram ao usuário — todos escaparam pelo mesmo motivo: cada peça estava correta isoladamente, e o defeito vivia na costura entre elas ou na ordem em que o usuário percorre a interface. Nenhum teste que chama funções Go diretamente exercitava esses caminhos:

1. O aviso de versão nova (Decisão 12) nunca aparecia na primeira abertura do menu — a checagem em segundo plano perdia a corrida contra o `survey.Select`, que assume o terminal antes do resultado chegar.
2. O seletor de pasta de destino, em `organize-pdf`, reabria no diretório do executável em vez de continuar a partir da pasta de origem recém-selecionada (Decisão 11).
3. Uma pasta de origem vazia só era percebida no fim de toda a calibração de regex, não no momento da seleção (Decisão 11).

Os três já foram corrigidos no código de produção — mas nenhum unitário existente teria pego a regressão se algum deles voltasse. **`go test ./...` cobre lógica pura; não cobre navegação interativa.** Isso é uma lacuna estrutural da suíte, não um descuido pontual — qualquer defeito que dependa de redesenho de tela, ordem de prompts ou timing de goroutine só aparece abrindo o programa de verdade.

**O que existe:**

- `internal/testcli/testcli.go` (`//go:build linux`): harness que abre o binário real dentro de um pseudo-terminal (`/dev/ptmx`, via `golang.org/x/sys/unix` — já era dependência do módulo, nenhuma foi acrescentada), envia teclas como um usuário faria (`Send`, `Down`, `Up`, `Enter`, `CtrlC`) e verifica o que apareceu na tela (`Expect`, `ExpectAny`, `NotExpect`, `Screen`). Remove sequências ANSI antes de comparar texto.
- `e2e/` (`//go:build e2e && linux`): os cenários em si. `TestMain` compila o binário uma única vez (a partir do código-fonte deste checkout, nunca um artefato publicado) e o reaproveita entre todos os testes do pacote.

**Decisão central: nenhum arquivo de produção foi alterado para viabilizar isso.** Um gancho de teste embutido no programa (ex: uma flag `--test-mode` ou um hook de sincronização) acabaria testando o gancho, não o caminho real que o usuário percorre — e é justamente esse caminho que vem falhando. O harness exercita o binário exatamente como é publicado.

**Como rodar:** `make e2e` (não está incluído em `make test`/`go test ./...` — são lentos, cada cenário inicia um processo e navega por prompts reais, e dependem de recursos específicos de Linux).

**Isolamento de ambiente:** cada sessão de teste roda com `XDG_CONFIG_HOME` apontando para um `t.TempDir()` (nunca toca em `~/.config/file-manager` de verdade) e com `PATH` sobrescrito para excluir qualquer Tesseract instalado na máquina — `TESSERACT_PATH` inválido sozinho não bastaria, porque `internal/ocr.NewTesseract()` cai em `exec.LookPath("tesseract")` quando `TESSERACT_PATH` está vazio ou inválido; só com o `PATH` também controlado é que `ocr.Available()` fica determinístico entre máquinas diferentes.

**Armadilha de sincronização a evitar em novos cenários:** `Expect` procura o texto em TODO o histórico acumulado da tela, não só no redesenho mais recente. Num fluxo que passa pelo mesmo prompt genérico mais de uma vez (ex: o seletor de pastas, chamado várias vezes em sequência em `organize-pdf`), usar um texto fixo que se repete a cada nível de navegação (ex: "PASTA DE ORIGEM") como alvo de `Expect` faz a chamada devolver na hora, lendo conteúdo antigo do buffer — sem esperar o redesenho de verdade. Na prática isso deixa uma tecla enviada em seguida correndo à frente da leitura do programa, podendo se perder. A correção é sempre esperar por um marcador que só existe, pela primeira vez, naquele passo específico — normalmente a linha `"Diretório atual: " + <caminho completo esperado>` — nunca um texto genérico que o prompt reimprime em todo redesenho. Cuidado também com colisão de prefixo: se um caminho A é prefixo de um caminho B já mostrado antes, esperar por A sozinho pode "casar" dentro do B antigo — prefira um marcador que não seja prefixo de nada que já apareceu.

**PDFs de fixture:** o gerador binário mínimo (`internal/pdfutil/integration_test.go`, função `buildTestPDF`) não pôde ser reaproveitado — vive em um arquivo `_test.go` de outro pacote, e arquivos `_test.go` não são importáveis por pacotes externos. A lógica foi copiada (não promovida a símbolo exportado de produção) para `e2e/pdf_fixture_test.go`; qualquer ajuste na fixture original deve ser replicado manualmente aqui se for relevante para os testes ponta a ponta.

## Processo de Release

Push de uma tag `v*` dispara `.github/workflows/release.yml`: o workflow extrai as notas de release em português para a tag, roda `go test ./...` como gate, compila os binários de Linux e Windows com `CGO_ENABLED=0` e publica o release com os dois artefatos anexados.

**Lançar uma versão nova**, depois que a entrada da versão em `CHANGELOG.md` **e** o arquivo `.github/release-notes/vX.Y.Z.md` já estiverem mesclados em `main`:
```bash
git tag -a vX.Y.Z -m "..."
git push origin vX.Y.Z
```
Nada mais é necessário — o workflow cuida de extrair as notas, testar, compilar e publicar.

A versão reportada por `file-manager version` vem da tag, injetada via `-ldflags` (o mesmo mecanismo de `main.version` descrito acima) — por isso a tag é a fonte da verdade da versão, não o código.

O job precisa de `permissions: contents: write` no workflow; sem isso a publicação do release falha com 403.

### Decisão: notas de release em português saem do próprio workflow, não são editadas depois

Até a v0.11.0 (14 releases publicados), `generate_release_notes: true` produzia só o changelog automático de commits, em inglês. As notas descritivas em português eram escritas **depois** da publicação, à mão, com `gh release edit <tag> --notes-file <arquivo>`. Levantamento dos releases recentes: em **seis dos oito mais recentes**, quem executava o release encerrava a sessão enquanto esperava o workflow terminar, e o release ficava temporariamente sem notas até alguém retomar manualmente; duas vezes a linha `**Full Changelog**` foi sobrescrita por acidente ao editar e precisou ser reconstruída via API.

O problema de fundo não era a espera — era que o texto voltado ao usuário final era produzido **depois** do release estar publicado, fora de qualquer PR, sem revisão nenhuma.

A partir desta mudança: `.github/extract-release-notes.sh <tag> <changelog> <rodapé>` monta as notas a partir de `.github/release-notes/<tag>.md` e anexa o rodapé fixo `.github/release-footer.md` (instruções de download, `chmod +x`, aviso do SmartScreen do Windows, lembrete de `file-manager update`). O workflow roda esse script como primeiro passo do job — **antes** de compilar — e falha o job se o arquivo não existir para a tag: publicar sem notas é pior que não publicar, e falhar antes de gastar minutos compilando é o comportamento certo.

O resultado vai para `softprops/action-gh-release` via `body_path`, com `generate_release_notes: true` mantido: pela documentação da action, quando `body`/`body_path` é fornecido junto com `generate_release_notes: true`, o texto fornecido é **pré-pendido** às notas geradas automaticamente pelo GitHub — não as substitui. É assim que a linha `**Full Changelog**` continua aparecendo no final de todo release, sem ninguém precisar preservá-la à mão.

### Decisão: `CHANGELOG.md` e as notas de release não compartilham texto, e não há fallback entre os dois

A primeira versão deste mecanismo usava a seção do `CHANGELOG.md` como fonte padrão, caindo em `.github/release-notes/<tag>.md` só "quando o changelog não bastasse". Testado na prática (`v0.9.0`), o resultado publicaria como nota de release um trecho como *"`internal/history.List` agora pula cada arquivo que falhar ao ler ou decodificar... passou a devolver `[]Header`... evita reter 60 mil entradas na memória"* — texto correto para quem mantém o código, incompreensível para quem baixa o `.exe` e dá duplo clique nele.

`CHANGELOG.md` e `.github/release-notes/<tag>.md` respondem a perguntas diferentes: o primeiro é o registro técnico (nomes de função, tipos, decisões de implementação) para quem mantém o projeto; o segundo é "o que muda para mim", para quem só usa o programa. Um fallback do segundo para o primeiro parece rede de segurança, mas garante que, no dia em que alguém esquecer de escrever as notas ao usuário, o release saia com o texto técnico — sem ninguém perceber, porque tecnicamente "funcionou". Por isso `.github/release-notes/<tag>.md` é **obrigatório, sem fallback**: na ausência dele, `extract-release-notes.sh` falha citando o caminho exato que falta criar, em vez de degradar em silêncio para o texto errado.

O efeito prático: quem escreve a feature escreve dois textos no mesmo PR — a entrada técnica em `CHANGELOG.md` (`[Não publicado]`) e as notas ao usuário em `.github/release-notes/<tag>.md` — revisados juntos, antes do release existir. Ver `docs/CONTRIBUTING.md`, seção "Processo de Release", para o passo a passo, e `.github/release-notes/README.md` para o que escrever em cada um (com exemplo lado a lado).

## Exportação de Documentação

A ferramenta consegue exportar sua própria documentação para uso em chatbots:

```bash
file-manager docs export --format context --output docs.md
file-manager docs export --format skill --output SKILL.md
```

- **Formato `context`:** Markdown detalhado com estrutura completa (útil para colar em chat de IA)
- **Formato `skill`:** Markdown com frontmatter YAML compatível com agentes de IA (`.md` pode ser instalado como skill)

Os metadados das FERRAMENTAS exportados (flags, exemplos, etc.) são gerados **automaticamente** a partir de `tool.Param`, reforçando a regra: uma única fonte de verdade.

### Comandos auxiliares (undo, profiles, update, version, docs export)

`internal/ui/docs.Render`/`Export` percorriam só `app.Tools()` — e por isso NUNCA incluíam
`undo`, `profiles` (e seus 4 subcomandos), `update`, `version` nem o próprio `docs export`,
porque nenhum deles é uma "ferramenta" do registry (não tem tela interativa própria nem
`Profile()`; existem só como comandos cobra montados diretamente em
`internal/app/root.go`/`internal/app/undo.go`). O efeito era o oposto do propósito do
recurso: quem instalava o `SKILL.md` e pedia para desfazer uma organização recebia
"não existe" ou uma invenção da IA — exatamente a alucinação que essa documentação existe
para impedir.

A correção: `internal/commanddocs.CommandDocs() []tool.Doc` documenta cada um desses
comandos com a mesma estrutura de `tool.Doc` usada pelas ferramentas (flags, exemplos,
"quando usar", avisos). `docs.Render`/`Export`/`NewScreen` agora recebem essa lista como
parâmetro extra, e os dois templates (`context.md.tmpl`, `skill.md.tmpl`) ganharam uma
seção "Comandos auxiliares" que os imprime com o mesmo nível de detalhe das ferramentas
(context) ou de forma concisa (skill).

`CommandDocs()` vive em pacote próprio (`internal/commanddocs`), não em `internal/app`
como seria mais natural: `internal/app` importa `internal/ui/mainmenu` (para abrir o menu
interativo), e `internal/ui/mainmenu` precisa da mesma lista para repassá-la à tela
`docs.NewScreen` — colocar `CommandDocs()` em `internal/app` criaria um ciclo de import
(`app` → `mainmenu` → `app`). `internal/commanddocs` só importa `internal/tool`, então
tanto `internal/app/root.go` quanto `internal/ui/mainmenu` podem importá-lo livremente.

**A lacuna passou despercebida porque nada verificava que a documentação cobrisse TODOS os
comandos** — `TestToolsConsistency` só olha `app.Tools()`. Agora `internal/app/command_docs_test.go`
tem `TestRootCommandsAreAllDocumented`, que percorre os subcomandos REAIS de
`NewRootCommand(...).Commands()` (recursivamente, entrando em comandos-grupo como
`profiles` e `docs`) e falha se algum comando folha não estiver documentado nem como
ferramenta nem em `CommandDocs()` — ignorando só `help` e `completion` (criados pelo
próprio cobra, nunca documentados em lugar nenhum do projeto). Um comando novo adicionado
a `NewRootCommand` no futuro **quebra este teste** até ganhar uma `tool.Doc` em
`internal/commanddocs`. `TestCommandDocsFlagsMatchCobra`, no mesmo arquivo, faz para os
comandos auxiliares o que `TestToolsConsistency` já fazia para as ferramentas: toda flag
documentada precisa existir de fato no comando cobra, e vice-versa.

## Armadilhas Conhecidas

### 1. `go.sum` e `pdfcpu`

O `pdfcpu` puxa dependências transitivas profundas (`golang.org/x/image/ccitt`, `hhrutter/tiff`, `x/crypto/ocsp`, `go-runewidth`) que um `go get` direto do módulo raiz **não resolve sozinho**.

Se aparecer erro "missing go.sum entry":

```bash
go mod download github.com/pdfcpu/pdfcpu
```

Isso acrescenta apenas hashes. **Nunca edite `go.sum` à mão** para "reverter" algo — quebra o build de todos os pacotes que dependem de survey ou go-isatty.

### 2. Trabalho Paralelo no Repositório

Ao rodar várias tarefas em paralelo (ex: múltiplos agentes escrevendo código Go):

- **Não faça `git add -A`** enquanto arquivos ainda estão sendo escritos
- O commit captura versões parciais → árvore que não compila

Espere que cada tarefa termine antes de stagear e commitar.

### 3. Sintaxe de Template em Scaffold

Os templates em `internal/scaffold/templates/` geram código Go cheio de chaves `{}`. Só construções `{{...}}` são do template Go:

```go
// No template:
// {{.ToolName}}   → substitui (template)
// {{"{{"}}        → emite {{{{ (literal, escapado)
// {{.Doc.Title}}  → substitui (template)
```

Para emitir `{{` literalmente, use `{{"{{"}}`.

### 4. Duplicação de Parâmetros

**Nunca** declare um parâmetro em múltiplos lugares:

❌ **Errado:**
```go
// options.go
type Options struct {
    Input string
}

// command.go params()
{Name: "input", ...}

// tool.go Doc()
Flags: []DocFlag{{Name: "input", ...}}  // ← duplicação
```

✅ **Correto:**
- Declarar em `params()` apenas
- `tool.BindAll()` registra a flag
- `tool.DocFlags()` gera a documentação

### 5. Screen.go Não é Testável Linha a Linha

A tela é **apenas I/O**. Não invista em testes unitários de `screen.go`. Teste a lógica de negócio em `internal/<dominio>util/`.

### 6. Perfis: ValidateName é Obrigatório

Sempre usar `config.ValidateName()` **antes** de construir caminhos de perfil. Proteção contra path traversal.

```go
// ✅ Correto
if err := config.ValidateName(name); err != nil {
    return err
}
path, err := config.ProfilePath(toolID, name)

// ❌ Errado (path traversal possível)
path := filepath.Join(dir, name + ".yaml")
```

### 7. Regex Multi-linha sobre Texto de OCR Exige `(?s)`

Em Go, `.` na expressão regular **não casa quebra de linha** por padrão. Texto vindo de OCR quebra linha o tempo todo, então qualquer regex que precise atravessar linhas precisa do prefixo `(?s)` (modo "dotall"). Caso real medido em `organize-pdf`: `MATRÍCULA.*?(\d{6,})` não casava contra texto de OCR; `(?s)MATRÍCULA.*?(\d{6,})` casou e classificou o arquivo corretamente. Sintoma no terreno: a regex "parece certa" (funciona num editor de regex genérico, que costuma ligar dotall por padrão) mas não casa nada dentro da ferramenta — o usuário desconfia de bug antes de suspeitar da própria regex.

## Próximos Passos Avaliados

Nesta seção ficam registradas **quatro melhorias já avaliadas e conscientemente adiadas** — nenhuma é urgente, e cada uma é documentada com a evidência que a motivou e o motivo de não ser urgente hoje. Sem esse registro, correm risco de virar uma lista de desejos sem nenhum contexto: daqui a alguns meses, ninguém lembraria por que existiam nem que problema concreto elas atacavam.

### 1. Verificação por Checksum no Auto-Atualizador

**O que é:** Hoje o comando `file-manager update` baixa o binário e, **antes de substituir**, executa o arquivo baixado com `version` para confirmar que roda. Isso protege contra download truncado ou corrompido. A melhoria seria adicionar validação criptográfica: o workflow de release publicaria um arquivo de somas (ex.: `SHA256SUMS`) junto dos binários, e o `update` conferiria a soma antes de trocar.

**Por que foi considerada:** O canal de comunicação (GitHub releases) e o armazenamento de artefatos (próprio repositório) são autenticados por TLS; a validação por execução (`VerifyBinary`) já cobre o caso realista (download incompleto ou corrompido durante a transmissão). O ganho da verificação criptográfica seria contra um cenário de comprometimento mais profundo: o servidor GitHub sendo invadido, ou o armazenamento de artefatos sendo alterado depois de publicado. Cenário plausível em teoria; raro na prática.

**Por que não é urgente:** O mecanismo existente de execução validante (`VerifyBinary`) funciona bem. A chance de um ataque sofisticado o bastante para alterar artefatos publicados mas não conseguir alterar o GitHub como um todo (ou a conexão HTTPS do usuário) é pequena o bastante para não jusitificar o trabalho hoje. Se publicar SHA256SUMS ficar na prioridade futura, considere também publicá-lo assinado com GPG.

### 2. Normalização do Texto Vindo de OCR

**O que é:** Uma etapa opcional de normalização de texto extraído por OCR, anterior à aplicação da regex de classificação. Colapsar espaços múltiplos, unificar caracteres notoriamente confundidos (ex: `0` ↔ `O`, `1` ↔ `l`), e possivelmente uma comparação tolerante a pequenas diferenças.

**Por que foi considerada — evidência medida:** Durante o desenvolvimento, observou-se em documentos reais que o OCR erra caracteres com frequência. Casos concretos: `ESCOLA` reconhecido como `ESCO` (caractere faltante), confusões entre `0` (zero) e `O` (letra O), entre `1` (um) e `l` (letra L minúscula). Consequência: expressões regulares calibradas sobre texto limpo falham sobre texto de OCR, e o arquivo cai em não-classificados sem que o usuário entenda o motivo.

**Por que não é urgente:** O teste de calibragem em `organize-pdf` já mostra quantos arquivos casariam **antes** de aplicar a regex, alertando sobre potencial descompasso. O relatório de execução informa o motivo de cada não-classificado, então o usuário percebe o problema em vez de ser surpreendido silenciosamente. Isso dá espaço para o usuário ajustar a regex manualmente (tornando-a mais tolerante) em vez de depender de normalização automática.

**Risco a considerar:** Unificar caracteres pode gerar **falsos positivos** — duas notas diferentes, com designações que diferem só num caractere facilmente confundido, virando a mesma chave de classificação. Por isso, se implementada futuramente, a normalização precisa ser **opcional** (flag ou perfil) e **conservadora** (nunca fazer unificação que quebre casos válidos).

### 3. Calibração com Mais de Uma Amostra

**O que é:** Hoje a calibração de expressões regulares em `organize-pdf` usa **um único** PDF de amostra. Se o lote tiver dois layouts diferentes, a regex calibrada na primeira amostra pode não servir para a segunda. A melhoria seria permitir escolher duas ou três amostras e exigir que a regex case em **todas** antes de seguir para a execução real.

**Por que foi considerada:** O cenário é realista — lotes heterogêneos com múltiplas versões de documento (ex: notas fiscais de emissores diferentes, cada um com layout próprio). Calibrar a regex contra a primeira variação e falhar na segunda é uma fonte previsível de erro.

**Por que não é urgente:** O teste de calibragem em `organize-pdf` já roda contra a pasta inteira (ou uma amostra escolhida do usuário) e mostra quantos arquivos casariam. Se a regex calibrada não pegar 100% dos arquivos, o cenário fica evidente no **simulador, antes** de qualquer arquivo ser movido — apenas um pouco mais tarde no fluxo do que seria com múltiplas amostras no passo de calibração. Quem receber esse resultado insatisfatório pode voltar e ajustar a regex manualmente para cobrir ambos os layouts.

### 4. Modo de Observação de Pasta

**O que é:** Um modo (`watch mode`) que monitora um diretório e processa automaticamente os arquivos que forem chegando, aplicando um perfil pré-salvo. Sem intervenção manual, os arquivos chegam, são classificados e saem organizados.

**Por que foi considerada — caso de uso recorrente:** O projeto foi desenvolvido motivado principalmente pelo cenário de lotes de notas fiscais. Na realidade operacional, essas notas não chegam todas de uma vez — elas chegam ao longo do mês. Um modo de observação resolveria essa fricção: dropar a pasta do dia na origem, deixar a ferramenta rodar à noite, acordar com os arquivos organizados no dia seguinte.

**Por que não é urgente:** Rodar a ferramenta manualmente sobre a pasta a cada dia (ou via agendador de sistema tipo `cron`) já resolve o caso de uso — é questão de conveniência, não de capacidade. A ferramenta faz hoje tudo que o caso de uso exige; o ganho seria de conforto do operador.

**Pontos a decidir antes de implementar:** (1) **Arquivo incompleto:** se um PDF ainda está sendo copiado para a pasta observada, há risco de processá-lo parcialmente. Precisa de uma estratégia — ex: ignorar arquivos modificados há menos de N segundos, ou só processar quando o `fstat` de tamanho não mudar por M segundos; (2) **Reprocessamento:** como evitar processar o mesmo arquivo duas vezes (ex: em caso de erro, ou se a configuração mudar)? Registrar um checksum? Um arquivo `.done`? Mover para um `_processed` depois?; (3) **Interrupção:** como o usuário interrompe a observação com segurança? Arquivo de flag? Sinal do SO? A implementação precisa garantir que um Ctrl+C no meio da observação deixa os arquivos em estado consistente.

## Exploração Adicional

- **docs/CONTRIBUTING.md** — Guia detalhado de contribuição com exemplos
- **README.md** — Uso de ferramentas, exemplos de linha de comando
- **internal/app/consistency_test.go** — Teste `TestToolsConsistency` que valida documentação vs. flags reais e integridade da registração de ferramentas
- **internal/tools/mergepdf/**, **splitpdf/**, **organizepdf/** — Exemplos de implementação completa

