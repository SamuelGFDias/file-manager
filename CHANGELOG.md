# Changelog

Todas as mudanças notáveis neste projeto são documentadas neste arquivo.

O formato é baseado em [Keep a Changelog](https://keepachangelog.com/pt-BR/1.0.0/), e este projeto segue [Versionamento Semântico](https://semver.org/lang/pt-BR/).

## Política de Versionamento

- **MAJOR:** Incrementado quando há mudanças incompatíveis nas flags de linha de comando (renomeação, remoção ou mudança semântica de uma flag existente) ou no formato de arquivo YAML de perfis. Quebra automações de quem já usa a ferramenta.
- **MINOR:** Incrementado quando são adicionadas novas ferramentas, novos modos, ou novas flags que não quebram compatibilidade com versões anteriores.
- **PATCH:** Incrementado para correções de bugs e melhorias internas que não afetam a interface externa.

A `1.0.0` é a declaração de estabilidade do projeto — o ponto em que a interface externa (flags, subcomandos, formato YAML de perfis) passa a ser considerada estável e suportada, não mais um bump de MINOR como os anteriores. **A partir dela**, a regra de MAJOR acima passa a valer no sentido usual do Versionamento Semântico: um incremento de MAJOR sinaliza quebra de compatibilidade com o que já está publicado. Antes da `1.0.0`, MINOR também podia trazer mudança de comportamento sem o mesmo compromisso de estabilidade — ver `AGENTS.md` para o detalhe do que fica congelado a partir desta versão.

## [Não publicado]

### Alterado

- **Interno:** o workflow de release (`.github/workflows/release.yml`) agora publica as notas em português automaticamente, extraídas por `.github/extract-release-notes.sh` da própria entrada do changelog (ou de `.github/release-notes/<tag>.md`, quando existir), em vez de exigir edição manual depois da publicação. Não afeta o comportamento do programa; ver `AGENTS.md`, seção "Processo de Release".

### Planejado

- [ ] Suporte para criptografia de perfis
- [ ] Modo batch para processing em lote via arquivo JSON de configuração
- [ ] Temas customizáveis para a interface interativa

## [1.0.0] - 2026-08-11

Primeira versão de produção: a interface externa (flags, subcomandos, formato YAML de perfis) passa a ser considerada estável — ver a nota na Política de Versionamento acima e a decisão correspondente em `AGENTS.md`. Não há mudança incompatível nesta versão; o motivo do MAJOR é a declaração de estabilidade em si, não uma quebra.

### Adicionado

- **`file-manager --version`/`-v` e o subcomando `version` agora também avisam quando há uma versão mais nova publicada** — antes esse aviso só existia no menu interativo. `--version` é o momento em que o usuário está perguntando exatamente "em que versão estou?"; é o momento natural de saber que existe uma mais nova, sobretudo quando ela corrige um defeito. Duas restrições não-negociáveis moldaram o desenho: (1) `--version` precisa continuar instantâneo e offline — uma consulta de rede síncrona o tornaria lento e dependente de conexão; (2) as três formas (`--version`, `-v`, `version`) produzem hoje saída byte a byte idêntica, garantido por teste, para que um script não quebre trocando de uma para outra. A solução para as duas: o aviso só aparece quando a saída padrão é um terminal (`ui.IsOutputTerminal()`, nova função em `internal/ui/term.go`, análoga a `IsInteractive()` mas sobre `os.Stdout` em vez de `os.Stdin` — a distinção importa porque `--version > arquivo` mantém o stdin de terminal, só o stdout é redirecionado). Redirecionado ou em pipe, a saída continua sendo, hoje como sempre, só a linha da versão, imediata e sem tocar rede. A linha da versão sempre sai primeiro (em stdout); o aviso, quando existe, vai inteiramente para `stderr` (`ui.WarnStderrf`/`ui.InfoStderrf`, novas variantes de `ui.Warnf`/`ui.Infof`), então capturar só stdout nunca traz o aviso junto — a garantia de saída idêntica entre as três formas continua valendo, agora explicitamente restrita a stdout. A consulta usa `selfupdate.Checker.WaitNotice` com um timeout de 1s (`internal/app/root.go`, menor que o 1,5s do menu — quem pede a versão espera resposta imediata); versão local não-semver (`dev`) não consulta nada. `--version`/`-v` deixaram de usar o mecanismo embutido do cobra (`root.Version`/`SetVersionTemplate`), que intercepta a flag antes de qualquer `RunE` e não dava gancho para imprimir o aviso depois da versão — o tratamento agora vive no `RunE` do comando raiz, chamando a mesma função `printVersion` usada pelo subcomando `version`, o que torna a igualdade de saída estrutural em vez de mantida por disciplina entre dois blocos de código.

### Corrigido

- **A documentação exportável para IA (`file-manager docs export --format context|skill`) não cobria `undo`, `profiles` (e seus subcomandos `list`/`export`/`import`/`path`), `update`, `version` nem o próprio `docs export`.** Causa: `docs.Render` percorria apenas `app.Tools()`, e nenhum desses comandos é uma "ferramenta" do registry — eles nunca tiveram como aparecer. O efeito era o oposto do propósito do recurso: quem instalasse o `SKILL.md` no seu agente de IA e pedisse para desfazer a última organização recebia "não existe" ou uma invenção, exatamente a alucinação que essa documentação existe para impedir. Agora `internal/commanddocs.CommandDocs()` documenta esses comandos com a mesma estrutura (`tool.Doc`) usada pelas ferramentas, `docs.Render`/`Export`/`NewScreen` passam a recebê-los como parâmetro, e os dois templates ganharam uma seção "Comandos auxiliares". Um teste novo (`TestRootCommandsAreAllDocumented`, em `internal/app/command_docs_test.go`) percorre os subcomandos reais de `NewRootCommand(...).Commands()` e falha se algum não estiver documentado nem como ferramenta nem como comando auxiliar — um comando novo, no futuro, quebra a suíte até ser documentado. Ver `AGENTS.md`, seção "Exportação de Documentação".

## [0.12.1] - 2026-08-11

### Corrigido

- **Seleção vazia de arquivos não é mais aceita em silêncio.** Relato real: ao usar `ocr-pdf` pelo menu, o usuário navegou até uma pasta com PDF, confirmou a tela de `survey.MultiSelect` sem marcar nenhum arquivo (nela, navegar é com as setas, **marcar é com a barra de espaço** — Enter só confirma) e o programa seguiu por mais seis perguntas (sufixo, idioma, sobrescrita, retomada, simulação) antes de falhar com `"informe ao menos um arquivo ou pasta em --input"`. Agora `filepicker.PickFiles` avisa na hora, lembrando da barra de espaço, e oferece tentar de novo (limite de 3 tentativas); `ocr-pdf` e `merge-pdf` — que tinham o mesmo laço de escolha de entradas duplicado — ganharam a mesma validação antecipada, garantindo que o fluxo nunca avança para as perguntas seguintes com zero entradas escolhidas. Uma pasta sem nenhum arquivo da extensão pedida também é avisada no ato (pasta e extensão citadas), em vez de apresentar uma lista vazia. Mesmo padrão já corrigido em `organize-pdf` na v0.2.1, que a ferramenta nova (`ocr-pdf`, v0.11.0) não tinha herdado.
- **A dica de `survey.MultiSelect` (usada para marcar vários arquivos) estava em inglês.** A tradução do template de `survey.Select` na v0.2.1 não cobriu o de escolha múltipla, que seguiu exibindo `[Use arrows to move, space to select, ...]` — a única parte em inglês de uma interface inteiramente em português, e exatamente a informação que faltou no relato acima. Agora `[use ↑ ↓ para navegar, ESPAÇO para marcar, → marca todos, ← desmarca todos, digite para filtrar, Enter para confirmar]`.

## [0.12.0] - 2026-08-11

### Adicionado

- **`file-manager --version` (e o atalho `-v`), além do subcomando `version` já existente.** `--version` é convenção praticamente universal em CLIs — quem digitava por reflexo recebia "flag desconhecida" e código de saída 1, a impressão de programa mal-acabado por uma lacuna de uma linha. As duas formas convivem: o subcomando não foi removido (quem já o usa, inclusive `internal/selfupdate.VerifyBinary`, continua funcionando sem mudança) e ambas imprimem **exatamente o mesmo texto**. O template padrão do cobra formataria `--version` de um jeito diferente do subcomando (`file-manager version v0.12.0 (...)` vs. `v0.12.0 (...)`) — ter duas saídas para a mesma informação quebraria qualquer script feito em cima de uma das duas. A descrição da flag em `--help` também sai em português (`mostra a versão do binário`), como o resto da ajuda.

## [0.11.0] - 2026-08-11

### Adicionado

- **Nova ferramenta `ocr-pdf`: transforma PDFs digitalizados em PDFs pesquisáveis de verdade.** Até aqui o reconhecimento de texto (Decisão 7 do AGENTS.md) só servia para leitura: o texto reconhecido ficava em memória, usado uma única vez para casar uma regex em `organize-pdf`/`split-pdf`, e o arquivo continuava sendo imagem — não pesquisável no Explorer do Windows nem em leitor de PDF, e cada execução reprocessava tudo do zero. `ocr-pdf` grava a camada de texto de volta no arquivo (`tesseract <imagem> <saida> -l <lang> pdf`), gerando um arquivo novo com sufixo (`--suffix`, default `-ocr`) — o original nunca é sobrescrito. Aceita arquivos avulsos ou pastas inteiras (`--input`, repetível, com a mesma semântica de `--max-depth` já usada por `merge-pdf`).
- **Regra de elegibilidade deliberadamente conservadora.** A abordagem reconstrói o PDF de saída a partir das imagens extraídas de cada página, o que é fiel quando a página é puro scan (uma única imagem, sem texto embutido) e destrutivo quando não é (imagem + texto, vetores, ou mais de uma imagem). Por isso `ocr-pdf` só processa arquivos cujas páginas sejam TODAS puro scan, recusando os demais com um motivo explícito — inclusive um arquivo que já tem texto embutido em todas as páginas (recusado por economia, não por erro: não há o que reconhecer). Perder conteúdo em silêncio seria muito pior que recusar o arquivo.
- **`--dry-run` classifica tudo sem gerar nada** — mostra quantos arquivos são elegíveis e quantos seriam pulados (e por quê) antes de comprometer minutos de processamento (~0,9s por página medido na prática; um lote de 200 documentos de 3 páginas leva quase 10 minutos). O progresso é impresso arquivo a arquivo (`[3/200] nota-003.pdf — 3 página(s)...`), e o processo é interrompível com Ctrl+C sem deixar lixo temporário.
- `--overwrite`/`--skip-existing` controlam o comportamento quando o destino já existe (o segundo pensado para retomar um lote grande interrompido no meio); `--report` grava um relatório CSV desta execução, no mesmo formato (BOM UTF-8, `sim`/`nao`) já usado por `organize-pdf --report` desde a v0.6.0.
- Exige o Tesseract instalado no sistema — ao contrário do OCR opcional de `organize-pdf`/`split-pdf` (que degrada normalmente sem ele), aqui é o próprio propósito da ferramenta: sem o Tesseract, `ocr-pdf` recusa rodar com um erro claro, mesmo em `--dry-run`.
- Custo de tamanho observado: um arquivo de 128 KB virou 241 KB — o Tesseract reescreve a imagem ao montar o PDF pesquisável, então o resultado é sempre maior que o original.

## [0.10.0] - 2026-08-11

### Adicionado

- **O aviso de nova versão distingue correção de novidade.** Antes, qualquer atualização disponível gerava o mesmo texto genérico. Agora o aviso (no menu principal e em `update`/`update --check`) tem três níveis: **novidade** ("nova versão disponível" — pode esperar), **correção** ("correção importante disponível" — a versão em execução tem um defeito conhecido já corrigido, continuar nela pode gerar resultado inconsistente) e **incompatibilidade** ("mudanças incompatíveis disponíveis" — recomenda ler as notas do release, com a URL, antes de atualizar). A classificação considera **todo o caminho** de releases entre a versão em execução e a mais recente, não só a mais recente: se a versão atual é `0.8.0` e a mais recente é `0.9.0`, o salto por si só parece só novidade, mas se `0.8.1` (uma correção) foi publicada no meio, ela já está incluída em `0.9.0` — e o usuário está exposto ao defeito que ela corrigiu. `internal/selfupdate.Releases` substitui a consulta a `/releases/latest` por `/releases` no fluxo de verificação, sem custo adicional de limite de API.

## [0.9.0] - 2026-08-11

### Corrigido

- **Um manifesto de histórico ilegível não derruba mais `undo --list` (nem o `undo` de nenhuma outra operação).** Antes, um único arquivo `~/.config/file-manager/history/*.yaml` corrompido — disco cheio no meio de uma gravação anterior, processo interrompido no momento errado — abortava a listagem inteira, inutilizando o `undo` de **todas** as operações registradas, inclusive as íntegras: o pior lugar possível para um ponto único de falha, já que "undo" é exatamente o recurso que existe para socorrer o usuário quando algo deu errado. `internal/history.List` agora pula cada arquivo que falhar ao ler ou decodificar, reporta um aviso em português citando o nome do arquivo e o motivo, e continua listando os demais. `err == nil` nesse caso — só um problema no PRÓPRIO diretório de histórico (ex: sem permissão) ainda propaga erro. `undo --list` e a tela interativa de desfazer exibem os avisos; a completação de `undo --id` (Tab) os ignora silenciosamente, de propósito — um Tab não pode cuspir aviso no meio da linha de comando.
- **`undo --list` não carrega mais o manifesto inteiro de cada operação para imprimir uma linha de resumo.** `internal/history.List` passou a devolver `[]Header` (metadados + `EntryCount`), não `[]Manifest` — com 200 execuções de 300 arquivos cada, isso evita reter 60 mil entradas na memória só para mostrar 200 linhas. `Load(id)` continua devolvendo o `Manifest` completo (com todas as `Entries`); é o que `undo` usa de fato para desfazer.

### Adicionado

- **Poda automática de histórico agora cobre também os manifestos PENDENTES (nunca desfeitos), não só os já desfeitos.** A poda introduzida nesta mesma versão de desenvolvimento (manifestos já desfeitos há mais de 30 dias) resolvia só uma fração do problema: quem organiza e nunca desfaz — o caso mais comum — continuava acumulando um manifesto por execução, para sempre. Agora um manifesto pendente com mais de 180 dias (`history.PrunePendingAfter`) também é removido automaticamente a cada `Save` (ex: a cada `organize-pdf` real). Um manifesto pendente mais novo que isso **nunca** é tocado, sob nenhuma condição — é exatamente ele que permite desfazer aquela operação mais tarde. Quando a poda remove algum manifesto pendente, `organize-pdf` avisa na hora (`Result.Details`): apagar em silêncio a capacidade de desfazer seria a surpresa que este projeto existe para evitar.
- **`undo --list` mostra por padrão só as 20 operações mais recentes**, com um rodapé (`mostrando 20 de 137 — use --all para ver todos`) quando há mais; nova flag `undo --all` mostra tudo. A tela interativa de desfazer (menu principal) segue o mesmo limite, com uma opção final "Ver operações mais antigas" que amplia para a lista completa — um `survey.Select` com centenas de itens era inutilizável.
- **`undo --prune` remove manualmente, na hora, os manifestos de histórico expirados** (mesmos critérios da poda automática), com confirmação a menos que `-y`. `--older-than <dias>` substitui os dois prazos padrão (30 dias para já desfeitas, 180 para pendentes) por um único limiar customizado.

## [0.8.1] - 2026-08-11

### Corrigido

- **OCR pulava páginas em silêncio quando o pdfcpu nomeava a imagem extraída com um prefixo diferente de `Im`.** O mapeamento de imagem extraída → número de página (`internal/pdfutil/textextract.go`) só reconhecia nomes no formato `..._Im<índice>.<ext>`; o pdfcpu deriva esse prefixo do nome do recurso XObject da página, que varia (`Im0`, `X0`, `Fm2`, ...), e num PDF real de duas páginas a segunda saiu como `X0` — sua imagem nunca ia para o Tesseract, e o usuário via "nenhum texto foi extraído" ou a expressão regular não casando, sem nenhuma pista da causa. O padrão agora aceita qualquer prefixo de letras. Existia desde a introdução do OCR (v0.2.0).
- **Falha silenciosa na extração de imagens agora gera aviso.** A causa raiz do defeito acima não era só a expressão regular: um arquivo extraído cujo nome não casasse com o padrão era descartado sem avisar ninguém — o que permitiu o bug atravessar seis versões sem ser notado. `ExtractPageTextsOpts`/`ExtractTextOpts` (`internal/pdfutil/textextract.go`) agora devolvem avisos não-fatais, propagados a `OrganizeResult.Warnings` e `SplitResult.Warnings`: um aviso por arquivo não reconhecido (citando o nome) e, se nenhuma imagem extraída puder ser associada a uma página, um aviso agregado explícito de que o pdfcpu pode ter mudado a convenção de nomes.

## [0.8.0] - 2026-08-11

### Adicionado

- **Completação de valores de flag no shell (Tab).** O cobra já completava nomes de comando e de flag; agora também completa **valores**. Enums fixos: `split-pdf --mode` (`page`/`range`/`regex`), `--ocr` em `split-pdf`/`organize-pdf` (`auto`/`always`/`never`), `organize-pdf --report-format` (`csv`/`json`), `merge-pdf --sort` (`name`/`mtime`). Valores dinâmicos, lidos de verdade do sistema: `undo --id` oferece só os identificadores de operações que ainda podem ser desfeitas (manifestos já desfeitos ficam de fora, para não levar a um erro evitável), `profiles list --tool` e `profiles export --tool` oferecem só as ferramentas que suportam perfis salvos, e `profiles import --file`/`organize-pdf --csv` filtram por extensão de arquivo (`yaml`/`yml` e `csv`) delegando ao cobra em vez de listar candidatos manualmente. `--ocr-lang` tenta listar os idiomas de fato instalados via `tesseract --list-langs`, com um limite de 300ms — sem o tesseract disponível, ou se ele não responder a tempo, cai na lista fixa conhecida (`por`, `eng`) em vez de travar a tecla Tab. `organize-pdf --csv-levels` lê o cabeçalho da planilha já apontada em `--csv` e oferece os nomes de coluna; sem `--csv` preenchido, ou com um arquivo que não existe, devolve lista vazia sem erro.
- **Nenhuma função de completação propaga erro, nunca.** Uma falha ao consultar perfis salvos ou histórico de operações (ex: diretório de configuração inacessível) resulta em lista vazia, jamais em mensagem de erro no meio da linha de comando — um Tab que cospe erro é pior do que um Tab que não completa nada.

### Alterado

- **O comando `completion` (acrescentado automaticamente pelo cobra) saiu da lista de comandos em `--help`.** É a única peça deste CLI em inglês, junto de `help` e do texto de `--help` — e aparecia em destaque para o usuário final que abre o `.exe` com duplo clique e nunca vai escrever uma linha de shell. Ele continua **funcionando normalmente** para quem o invoca diretamente (`file-manager completion zsh`); só a funcionalidade de esconder foi usada (`CompletionOptions.HiddenDefaultCmd`), nunca a de desativar (`DisableDefaultCmd`), que prejudicaria quem usa terminal sem beneficiar ninguém.
- O comando `help`, o texto da flag `--help` e os **rótulos estruturais da ajuda** (`Usage:`→`Uso:`, `Available Commands:`→`Comandos disponíveis:`, `Flags:`→`Opções:`, `Global Flags:`→`Opções globais:`, entre outros, via `SetUsageTemplate`) foram traduzidos para português — antes, a ajuda saía com descrições em português e rótulos estruturais em inglês, o que lia pior do que não traduzir nada. Dois resquícios continuam em inglês por não terem ponto de customização exposto pelo cobra: o `[flags]` na linha "Uso:" (vem de um método Go interno, não do template) e mensagens de erro do próprio cobra (ex: comando desconhecido). O texto de cada subcomando do `completion` (`completion bash`, `completion zsh`, etc.) também continua em inglês — não é razoavelmente configurável sem reescrever lógica interna do cobra, e o custo é baixo já que o comando está escondido de `--help`.

## [0.7.0] - 2026-08-11

### Adicionado

- **Organizar PDFs a partir de uma planilha (`organize-pdf --csv`).** Até aqui a hierarquia de pastas saía do conteúdo de cada PDF, por regex (`--level`). O caso inverso é comum: o usuário já tem uma planilha dizendo onde cada documento deve ser arquivado, e o PDF fornece só a **chave** para procurar nela. `--csv <planilha>` inverte a fonte da hierarquia: a primeira coluna da planilha (ou `--csv-key-column`) casa com a chave extraída do PDF via `--csv-key-regex` (obrigatório junto com `--csv`), e as demais colunas (ou `--csv-levels`, na ordem informada) formam os níveis de pasta. O nome do arquivo de destino é a própria chave por padrão; `--filename-regex`, quando informado, continua valendo e sobrepõe. `--csv` é incompatível com `--level` — a hierarquia vem de um ou de outro, nunca dos dois.
- **Os nomes de pasta gerados a partir da planilha são normalizados: acentos são removidos e espaços viram `_`** (`pdfutil.NormalizeComponent`, novo `internal/pdfutil/csvmap.go`) — "São Gonçalo" vira `Sao_Goncalo`, "Niterói" vira `Niteroi`. Decisão intencional: nome de pasta acentuado dá problema em rede compartilhada e em ambiente misto Windows/Linux.
- **A leitura da planilha (`pdfutil.LoadCSVMap`) aceita separador vírgula ou ponto e vírgula, detectado automaticamente pela primeira linha** — o Excel em português salva CSV com `;` por padrão, e é essa a planilha que a maioria dos usuários vai ter na mão. O BOM UTF-8 que o Excel (e o próprio `--report` desde a v0.6.0) costuma gravar no início do arquivo é descartado automaticamente.
- **Chaves são comparadas como texto, com espaços das pontas removidos — nunca convertidas para número:** `001` e `1` são chaves diferentes, porque zeros à esquerda são significativos em número de nota. Uma **chave duplicada na planilha é erro**, citando a chave repetida: duas linhas apontando para pastas diferentes sob a mesma chave precisam ser corrigidas por quem preencheu a planilha, não resolvidas por sorteio. Uma **célula de nível vazia, ao contrário, não é erro:** o componente de pasta correspondente é só omitido, com um aviso no resultado citando a chave e a coluna.
- **Uma chave que o regex encontra no PDF mas que não existe na planilha não interrompe o lote** — é, na prática, o caso mais comum: o arquivo vai para `--unclassified-dir` com o motivo citando a chave encontrada (ex.: `chave "999" não está na planilha`), para conferir na planilha depois.
- No fluxo interativo, a etapa de hierarquia de pastas agora pergunta primeiro "Pelo conteúdo de cada PDF" ou "Por uma planilha CSV"; o caminho CSV mostra um resumo (linhas, coluna-chave, colunas de hierarquia e um exemplo de caminho gerado) antes de qualquer processamento, permite trocar a coluna-chave ou reordenar as colunas de hierarquia, e calibra a regex da chave reaproveitando o mesmo componente de calibração por exemplo (`internal/ui/calibrate`) usado pelo modo por conteúdo.
- `--csv`, `--csv-key-regex`, `--csv-key-column` e `--csv-levels` são persistidos no perfil salvo, como as demais opções de `organize-pdf`.

## [0.6.0] - 2026-08-11

### Adicionado

- **Relatório da organização (`organize-pdf --report`).** O resultado de uma organização aparecia resumido na tela e desaparecia quando o terminal fechava. Num lote de notas fiscais isso não basta: é preciso poder conferir depois por que cada arquivo foi parar onde foi, e quais não foram classificados. `--report <caminho>` grava um arquivo com uma linha por arquivo considerado, classificado ou não, incluindo o motivo da não-classificação (ex.: `nível "fornecedor" não encontrado`). `--report-format` escolhe entre `csv` (padrão) e `json`.
- **O relatório também é gerado com `--dry-run`** — é justamente aí que ele mais serve, permitindo conferir a classificação inteira numa planilha antes de tocar em qualquer arquivo de verdade.
- O CSV sai com **BOM UTF-8**, porque o público desta ferramenta abre o relatório no Excel em português — sem o BOM os acentos chegam corrompidos. As linhas saem sempre ordenadas por nome de arquivo, para que duas execuções do mesmo lote sejam comparáveis. Assim como o manifesto de histórico (v0.5.0), uma falha ao gravar o relatório nunca falha a organização, que já aconteceu: vira um aviso no resultado.
- Novo arquivo `internal/pdfutil/report.go`: `BuildReport` (função pura), `WriteReportCSV` e `WriteReportJSON`. `Options.Report`/`Options.ReportFormat` são persistidos no perfil salvo, ao contrário de `dry_run`/`sample`.

### Corrigido

- **`--dry-run` podia prometer uma classificação que a execução real desmentia, em caso de colisão de destino.** Dois arquivos do mesmo lote que resolvem para o mesmo destino (nota fiscal duplicada na pasta de entrada, ou o mesmo número de nota em fornecedores diferentes — comum no dia a dia de quem organiza nota fiscal) só eram detectados como colisão na execução real, e só por acidente: como o primeiro arquivo já tinha sido gravado em disco quando o segundo chegava, um `os.Stat` pensado para detectar colisão com uma execução ANTERIOR também pegava esse caso "por tabela". A simulação, que nunca grava nada, nunca via essa colisão — o CSV de `--dry-run` mostrava os dois arquivos como classificados, enquanto a execução real reclassificava o segundo para `--unclassified-dir`. `Organize` agora detecta as duas formas de colisão (destino já reivindicado por outro arquivo do mesmo lote, e destino já existente em disco de uma execução anterior) com a mesma checagem explícita, em `--dry-run` e em execução real — as duas produzem o mesmo `Organized`/`Unclassified`, e portanto o mesmo relatório, sobre a mesma entrada.
- A linha de detalhe de um arquivo não classificado por colisão de destino dizia `nível "destino" não encontrado` — "destino" nunca foi um nível calibrado pelo usuário, e a mensagem dava a entender erro de calibração. Agora diz `destino já existe: <caminho>`.

### Adicionado

- **Desfazer uma organização (`organize-pdf`).** `organize-pdf` copia por padrão, mas com `--move` a operação era irreversível — quem roda um lote grande com uma regex recém-calibrada erra pelo menos uma vez, e até aqui esse erro custava reorganizar tudo à mão. Toda execução real (nunca uma simulação com `--dry-run`) agora grava um manifesto do que foi copiado ou movido, incluindo os arquivos que caíram em `--unclassified-dir`. Novo comando `file-manager undo [--id <id>] [--last] [--dry-run] [-y|--yes] [--list] [--force]`: desfazer uma cópia apaga o que foi criado no destino (o original nunca é tocado); desfazer um movimento devolve os arquivos à origem. Sem `--id` nem `--last`, em terminal interativo pergunta qual operação desfazer; sem terminal, falha com mensagem clara. O menu principal ganhou a opção "Desfazer uma organização", que só aparece depois de pelo menos uma operação real registrada.
- **As regras de segurança são o núcleo da feature.** Nada fora do manifesto é tocado; um arquivo cujo tamanho mudou desde a organização original é pulado e reportado, nunca apagado (pode ter sido editado depois — a verificação é por tamanho, não por conteúdo, por custo num lote grande); uma origem já ocupada não é sobrescrita ao devolver um arquivo movido; e só diretórios de fato vazios são removidos no destino, nunca de forma recursiva. Um manifesto já desfeito recusa ser desfeito de novo sem `--force`.
- Novo pacote `internal/history`: grava e lê o manifesto (`Save`, `Load`, `List`, `MarkUndone`) e implementa a lógica de desfazer (`Undo`). A gravação é injetada em `pdfutil.OrganizeOptions.Recorder`, para que `internal/pdfutil` continue sem depender de configuração do usuário — só quem chama `Organize` (o comando `organize-pdf`) conhece os dois domínios. Simulação (`--dry-run`) nunca gera manifesto: nada foi alterado, não há o que desfazer. Falha ao gravar o manifesto nunca falha a operação de organizar, que já aconteceu — vira um aviso no resultado.

## [0.4.0] - 2026-08-11

### Adicionado

- **Exportação e importação de perfis.** Quem calibra as regras de um perfil e quem usa a ferramenta no dia a dia costumam ser pessoas diferentes, em máquinas diferentes — até aqui o perfil calibrado ficava preso ao diretório de configuração da máquina onde foi criado, sem nenhum caminho para sair de lá. Novo comando `file-manager profiles export --tool <id> --name <perfil> --output <arquivo.yaml>` grava o perfil num arquivo; `file-manager profiles import --file <arquivo.yaml> [--name <novo-nome>] [--force]` lê esse arquivo de volta, valida que a ferramenta declarada existe e suporta perfis, e valida o conteúdo contra as opções dessa ferramenta antes de gravar — um arquivo corrompido ou de versão incompatível falha na importação, não no meio de um lote processado com dados errados. Também novos: `file-manager profiles list [--tool <id>]` (agrupado por ferramenta) e `file-manager profiles path` (mostra onde os perfis ficam guardados). A tela interativa de perfis ganhou as ações "Exportar para arquivo" e "Importar de arquivo" no mesmo menu usado por todas as ferramentas.
- O arquivo exportado é exatamente o mesmo envelope YAML usado internamente para persistir perfis — exportação e importação são simétricas por construção, sem um segundo formato a manter sincronizado. Novas funções em `internal/config`: `ExportProfile`, `ReadProfileFile`, `ImportProfile`.

## [0.3.1] - 2026-08-11

### Corrigido

- **Aviso de versão nova no menu principal agora aparece na primeira abertura.** A verificação roda em segundo plano e leva cerca de 250ms, mas o menu consultava o resultado uma única vez, no instante da abertura, quando ainda não havia chegado — e o seletor assume o terminal logo em seguida, sem redesenhar o menu. Na prática o aviso só apareceria se o usuário entrasse numa ferramenta e voltasse. `selfupdate.Checker` ganhou `WaitNotice(timeout)`, que aguarda no máximo o timeout pelo resultado; o menu usa `WaitNotice(1,5s)` só na primeira renderização e `Notice()` (não bloqueante) nas seguintes. Sem internet ou se o tempo estourar, o menu abre normalmente e nada é exibido.

## [0.3.0] - 2026-08-11

### Adicionado

- **Auto-atualização: novo subcomando `update`.** Consulta o último release publicado no GitHub, compara com a versão em execução e, se houver uma mais nova, mostra a progressão (ex: `v0.1.0 → v0.2.1`) e o link do release antes de pedir confirmação. `-y`/`--yes` atualiza sem perguntar; `--check` só verifica, sem baixar nem substituir nada. Já na última versão, informa e sai sem erro. Build local (versão `dev`) é detectado e avisado. Antes de substituir o executável em uso, o binário baixado é executado para validação — um download corrompido aborta a troca sem tocar no executável atual. Novo pacote `internal/selfupdate`, sem dependências externas (só biblioteca padrão do Go).
- **Aviso de versão nova no menu principal.** Quando há uma versão mais recente publicada, o menu exibe um aviso com a progressão de versão e o comando para atualizar. A verificação roda em segundo plano, uma única vez por sessão, e nunca bloqueia a abertura do menu. Sem internet, o menu abre normalmente e nenhum aviso aparece — falha de rede é silenciosa, nunca um erro na tela. Build local (`dev`) não gera aviso.

### Modificado

- **Interface mais legível.** No menu principal, a descrição de cada ferramenta agora aparece só na opção destacada (acompanhando a seta), em vez de todas ao mesmo tempo — feito sobrescrevendo o template de seleção do `survey`. A dica `[Use arrows to move, type to filter]`, única parte da interface em inglês, foi traduzida para `[use ↑ ↓ para navegar, digite para filtrar, Enter para confirmar]`. Novos helpers visuais em `internal/ui` (`Bold`, `Highlight`, `PathText`, `Count`, `Step`, `Divider`, `Blank`). O fluxo de `organize-pdf` passou a mostrar "Passo N de 7" em cada etapa, com separação entre blocos e destaque em caminhos e quantidades — sem alterar nenhum texto de pergunta nem a ordem das etapas.

## [0.2.1] - 2026-08-11

### Corrigido

- **`organize-pdf`: pasta de origem sem PDFs agora é avisada na hora.** Antes, o usuário só descobria a pasta errada no final, depois de toda a calibração de regex, com um "0 de 0 arquivos" sem explicação. Agora a contagem de PDFs acontece no ato da seleção da pasta de origem: zero PDFs bloqueia o avanço e oferece escolher outra pasta (limite de 5 tentativas); com PDFs, confirma de imediato quantos foram encontrados.
- **`organize-pdf`: aviso quando o PDF de amostra está fora da pasta de origem**, com confirmação explícita (default: não) antes de seguir em frente — foi assim que o usuário calibrou as regras contra um documento que não fazia parte do lote a processar.
- **`organize-pdf`: resumo antes de aplicar**, com caminhos absolutos de origem e destino, contagem de PDFs, e se a operação vai copiar ou mover.
- **`organize-pdf`: prompts de seleção agora identificam a etapa** (`PASTA DE ORIGEM`, `PASTA DE DESTINO`, `PDF de AMOSTRA`) em vez do genérico "Selecione um diretório" nas duas etapas.
- **Seletores de pasta encadeados não voltam mais ao diretório do executável.** Depois de escolher a pasta de origem, o prompt de destino reabria em `~/.file_manager` (pasta do binário), que não tem subpastas e por isso parecia vazia — o usuário concluía que a seleção não tinha funcionado e refazia toda a navegação. O prompt de destino agora começa na pasta de origem recém-selecionada, e o `filepicker` ganhou memória do último diretório usado (`LastDir()`/`resolveStart()`), respeitada sempre que o chamador não passa um `start` explícito.

### Notas

- Descoberta relacionada, documentada em `AGENTS.md` e no `README.md`: em Go, `.` na regex não casa quebra de linha, e texto de OCR quebra linha com frequência — regex que precisam atravessar linhas exigem o prefixo `(?s)`.

## [0.2.0] - 2026-08-11

### Adicionado

- **Suporte a OCR** para PDFs digitalizados (sem camada de texto): quando `split-pdf --mode regex` ou `organize-pdf --level` encontram uma página sem texto embutido, a imagem da página é extraída pelo pdfcpu e lida via [Tesseract](https://github.com/tesseract-ocr/tesseract) (executável externo, invocado por `os/exec` — não é binding CGO, então o binário continua estático e cross-compilável para Windows).
- **Flags `--ocr` e `--ocr-lang`** em `split-pdf` e `organize-pdf`: `--ocr` controla o modo (`auto`, `always` ou `never`; default `auto`) e `--ocr-lang` o idioma do OCR (default `por`). Persistidas nos perfis YAML como `ocr` e `ocr_lang`.
- **Novo pacote `internal/ocr`:** wrapper do executável `tesseract`, com detecção automática (variável `TESSERACT_PATH`, PATH e, no Windows, os caminhos usuais de instalação) e aviso de instalação por sistema operacional quando ausente.
- Em `organize-pdf`, a calibração interativa passa a usar o mesmo `TextOptions` do processamento real, garantindo que a regex calibrada veja o mesmo texto (nativo ou de OCR) que a execução vai processar.

### Notas

- Sem o Tesseract instalado, nada quebra: a execução segue normalmente com um aviso, sem OCR.
- O OCR custa aproximadamente 1 segundo por página.
- O reconhecimento de caracteres não é perfeito (ex: `ESCOLA` pode virar `ESCO`, `0` pode ser confundido com `O`) — regex sobre conteúdo que pode ter passado por OCR devem ser tolerantes a esse tipo de erro.

## [0.1.0] - 2026-08-11

### Adicionado

- **Menu interativo:** Interface de navegação em pilha com seleção de ferramentas via cursor e teclado.
- **Ferramenta `merge-pdf`:** Une múltiplos PDFs com suporte a recursão em pastas, ordenação por nome ou data de modificação.
- **Ferramenta `split-pdf`:** Separa PDFs em três modos: página individual, intervalos definidos ou padrão regex no conteúdo (com captura para nomes).
- **Ferramenta `organize-pdf`:** Organiza PDFs em hierarquia de pastas com base em regras regex multi-nível; funciona também como renomeador em lote.
- **Sistema de perfis:** Salva e carrega configurações em YAML de forma persistente (plataforma: `%AppData%\file-manager\profiles` no Windows, `~/.config/file-manager/profiles` em Linux/macOS).
- **Exportação de documentação:** Dois formatos de saída para documentar a ferramenta em contextos de IA:
  - Formato `context`: Markdown detalhado para colar em chat.
  - Formato `skill`: YAML com frontmatter para instalar em agentes de IA.
- **Gerador de scaffold:** Comando `make new-tool NAME=...` para criar uma ferramenta nova com estrutura de arquivos pré-pronta.
- **Build cross-platform:** Makefile com alvos para compilação nativa (Linux, Windows) com `CGO_ENABLED=0` garantindo distribuição como binário único sem dependências de runtime.
- **Testes:** Cobertura de testes para lógica de domínio (merge, split, organize, calibração de regex) com abordagem de testes de tabela.
- **Linting e formatação:** Targets `make lint` e `make fmt` com suporte opcional a golangci-lint.

### Removido

- Nenhuma mudança em versão 0.1.0 (lançamento inicial).

---

**Nota de compatibilidade:** Esta é a versão inicial. Recomenda-se que usuários dependam de perfis (salvar via interface ou arquivos YAML) para suas automações, garantindo portabilidade entre versões MINOR e PATCH futuras.
