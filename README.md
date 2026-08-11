# file-manager

Um conjunto de ferramentas de linha de comando para gerenciar, manipular e organizar arquivos PDF com precisão e automação.

## Instalação

### Compilar do código-fonte

```bash
make build
```

O binário será gerado em `dist/file-manager`.

Para compilar para outras plataformas:

```bash
make build-all  # Compila para Linux e Windows (amd64)
```

### Baixar o binário pré-compilado

Veja a seção de releases do repositório para baixar o binário pré-compilado para sua plataforma.

Uma vez instalado, atualizar para a versão mais nova é só rodar `file-manager update` — veja a seção [Atualização](#atualização) abaixo.

## Uso rápido

Para abrir o menu interativo com todas as ferramentas disponíveis:

```bash
file-manager
```

Cada ferramenta também funciona com flags de linha de comando. Use `file-manager <ferramenta> --help` para ver as opções de uma ferramenta específica.

## Ferramentas

### merge-pdf

Une múltiplos PDFs em um único arquivo.

| Flag | Tipo | Descrição |
|------|------|-----------|
| `-i, --input` | string (repetível) | Arquivo ou pasta de entrada (pode ser usado várias vezes) |
| `-o, --output` | string | Caminho do arquivo PDF de saída |
| `--max-depth` | int | Profundidade máxima de recursão em pastas (default: 1; 0 = só a pasta atual; -1 = ilimitado) |
| `--sort` | string | Ordenação dos arquivos: `name` ou `mtime` (modificação) |
| `--overwrite` | bool | Sobrescrever arquivo de saída se existir |

**Exemplos:**

```bash
file-manager merge-pdf --input ~/documentos/contratos --output ~/merged.pdf --sort name
```

```bash
file-manager merge-pdf -i arquivo1.pdf -i arquivo2.pdf -i arquivo3.pdf -o resultado.pdf --overwrite
```

### split-pdf

Separa um PDF em múltiplos arquivos. Suporta três modos: por página individual, por intervalos ou por padrão regex no conteúdo.

| Flag | Tipo | Descrição |
|------|------|-----------|
| `-i, --input` | string | Arquivo PDF de entrada |
| `-o, --output-dir` | string | Diretório de saída dos PDFs separados |
| `--mode` | string | Modo de separação: `page` (um por página), `range` (intervalos) ou `regex` (padrão no texto) |
| `--ranges` | string | Intervalos (para mode=range): ex. `1-5,6-10,15` |
| `--regex` | string | Padrão regex (para mode=regex); grupo de captura vira parte do nome do arquivo |
| `--name-template` | string | Template do nome dos arquivos de saída (variáveis: `{{.Index}}`, `{{.Match}}`) |
| `--overwrite` | bool | Sobrescrever arquivos de saída se existirem |
| `--ocr` | string | Uso de OCR em PDFs sem texto (só afeta `--mode regex`): `auto`, `always` ou `never` (default: `auto`) |
| `--ocr-lang` | string | Idioma do OCR (ex: `por`, `eng`) (default: `por`) |

**Exemplos:**

```bash
file-manager split-pdf --input documento.pdf --output-dir ./paginas --mode page
```

```bash
file-manager split-pdf -i contrato.pdf -o ./separados --mode range --ranges 1-10,11-20 --name-template "parte_{{.Index}}.pdf"
```

```bash
file-manager split-pdf -i faturas.pdf -o ./faturadas --mode regex --regex "Fatura #(\d+)" --name-template "fatura_{{.Match}}.pdf"
```

### organize-pdf

Organiza PDFs de uma pasta em uma hierarquia de subpastas com base no conteúdo de cada arquivo. Usa expressões regulares para classificar e criar estruturas de diretórios.

| Flag | Tipo | Descrição |
|------|------|-----------|
| `-i, --input` | string | Pasta contendo os PDFs a organizar |
| `-o, --output` | string | Pasta de destino da organização |
| `--level` | string (repetível) | Definição de nível: `'rótulo=regex'`; cria um nível de pasta para cada correspondência |
| `--filename-regex` | string | Regex para extrair nome do arquivo (grupo de captura vira nome) |
| `--move` | bool | Mover arquivos (padrão: copiar) |
| `--unclassified-dir` | string | Diretório para PDFs que não correspondem a nenhuma regra (default: `_unclassified`) |
| `--dry-run` | bool | Simular a operação sem executar |
| `--sample` | int | Testar com apenas N arquivos (útil antes de processar tudo) |
| `--overwrite` | bool | Sobrescrever arquivos existentes |
| `--ocr` | string | Uso de OCR em PDFs sem texto embutido: `auto` (só quando não há texto), `always` ou `never` (default: `auto`) |
| `--ocr-lang` | string | Idioma do OCR (ex: `por`, `eng`) (default: `por`) |
| `--report` | string | Caminho do arquivo de relatório desta execução; vazio (default) = não gera |
| `--report-format` | string | Formato do relatório: `csv` ou `json` (default: `csv`) |
| `--csv` | string | Planilha que define a hierarquia de pastas de destino; vazio (default) = hierarquia vem do conteúdo do PDF (`--level`). Incompatível com `--level` |
| `--csv-key-regex` | string | Regex que extrai do PDF a chave usada para procurar na planilha (`--csv`). Obrigatório junto com `--csv` |
| `--csv-key-column` | string | Coluna da planilha (`--csv`) com a chave; vazio = primeira coluna do cabeçalho |
| `--csv-levels` | string (repetível) | Colunas da planilha (`--csv`) que formam a hierarquia, na ordem; vazio = todas menos a chave, na ordem do arquivo |

**Exemplos:**

```bash
file-manager organize-pdf --input ~/documentos --output ~/organizado \
  --level 'ano=(\d{4})' \
  --level 'mes=(?:Janeiro|Fevereiro|Março|...)' \
  --move --dry-run
```

```bash
file-manager organize-pdf -i ./invoices -o ./invoices_organized \
  --level 'cliente=(?:ClienteA|ClienteB|ClienteC)' \
  --level 'ano=(\d{4})' \
  --filename-regex 'invoice_(\w+_\d+)'
```

Com zero `--level`, a ferramenta funciona como um renomeador em lote dos PDFs.

## Organizar a partir de uma planilha

Até aqui a hierarquia de pastas sai do conteúdo de cada PDF, por regex. O caso inverso é comum: o usuário já tem uma planilha dizendo onde cada documento deve ser arquivado, e o PDF só precisa fornecer a **chave** para procurar nela.

Planilha de exemplo (`planilha.csv`):

```
NOTA,CIDADE,BAIRRO
001,São Gonçalo,Laranjal
003,Rio de Janeiro,Centro
005,Niterói,Fonseca
```

```bash
file-manager organize-pdf --input ./notas --output ./organizado \
  --csv ./planilha.csv --csv-key-regex "NOTA:\s*(\d+)"
```

Resultado:

```
organizado/
  Sao_Goncalo/
    Laranjal/
      001.pdf
  Rio_de_Janeiro/
    Centro/
      003.pdf
  Niteroi/
    Fonseca/
      005.pdf
```

`--csv-key-regex` extrai do texto de cada PDF a chave (aqui, o número da nota); a primeira coluna da planilha (ou `--csv-key-column`, se informado) casa com essa chave, e as demais colunas (ou `--csv-levels`, na ordem informada) formam os níveis de pasta. O nome do arquivo de destino é a própria chave, por padrão — `--filename-regex`, quando informado, continua valendo normalmente e sobrepõe a chave.

**Os nomes de pasta são normalizados: acentos são removidos e espaços viram `_`.** "São Gonçalo" vira `Sao_Goncalo`, "Niterói" vira `Niteroi`. É intencional: nome de pasta acentuado dá problema em rede compartilhada e em ambiente misto Windows/Linux.

**A planilha aceita separador vírgula ou ponto e vírgula, detectado automaticamente pela primeira linha** — o Excel em português salva CSV com `;` por padrão, e é essa a planilha que a maioria dos usuários vai ter na mão. O BOM UTF-8 que o Excel costuma gravar no início do arquivo é descartado automaticamente.

**Chaves são comparadas como texto, com espaços das pontas removidos — nunca convertidas para número**: `001` e `1` são chaves diferentes, porque zeros à esquerda são significativos em número de nota. Uma chave duplicada na planilha é erro (citando a chave repetida): duas linhas apontando para pastas diferentes sob a mesma chave precisam ser corrigidas por quem preencheu a planilha, não resolvidas por sorteio. Uma célula de nível vazia, ao contrário, não é erro: o componente de pasta correspondente é só omitido, com um aviso no resultado citando a chave e a coluna.

**Uma chave que o regex encontra no PDF mas que não existe na planilha não interrompe o lote** — é, na prática, o caso mais comum: o arquivo vai para `--unclassified-dir` com o motivo citando a chave encontrada (ex.: `chave "999" não está na planilha`), para conferir na planilha depois.

`--csv` é incompatível com `--level` (a hierarquia vem de um ou de outro, nunca dos dois) e exige `--csv-key-regex`; as flags `--csv-key-column` e `--csv-levels` só têm efeito junto com `--csv`. `--dry-run`, `--report`, `--overwrite`, `--move`/copiar e o restante do comportamento de `organize-pdf` funcionam exatamente igual em modo `--csv`.

## Relatório da organização

O resultado de uma organização aparece resumido na tela e some quando o terminal fecha. Num lote de notas fiscais isso não basta: mais cedo ou mais tarde alguém vai precisar conferir por que uma nota específica foi parar em determinada pasta, ou quais notas não foram classificadas e por quê — é rastreabilidade, não conveniência.

```bash
file-manager organize-pdf --input ./notas --output ./organizado \
  --level "fornecedor=FORNECEDOR:\s*(\w+)" --report ./relatorio-organizacao.csv
```

`--report <caminho>` grava um arquivo com **uma linha por arquivo considerado**, classificado ou não, com estas colunas: `arquivo, origem, destino, classificado, motivo`. Para um arquivo não classificado, `destino` fica vazio e `motivo` explica o porquê (ex.: `nível "fornecedor" não encontrado`, `nome do arquivo não encontrado`). `--report-format` escolhe entre `csv` (default, para abrir numa planilha) e `json` (para quem for processar o relatório por programa).

**Funciona junto com `--dry-run`** — e é justamente aí que ele mais serve: gera o relatório completo, com a classificação de cada arquivo, sem copiar ou mover nada, para conferir numa planilha antes de aplicar de verdade. A simulação inclusive detecta colisão de destino (dois arquivos do lote resolvendo para o mesmo caminho, ou um destino que já existe de uma execução anterior) exatamente como a execução real detectaria — rodar com `--dry-run` e sem, sobre a mesma entrada, produz o mesmo relatório.

```bash
file-manager organize-pdf --input ./notas --output ./organizado \
  --level "fornecedor=FORNECEDOR:\s*(\w+)" --dry-run --report ./relatorio-organizacao.csv
```

O CSV é gravado com **BOM UTF-8** no início do arquivo: sem ele, o Excel em português (o programa mais provável para abrir este relatório) interpreta o conteúdo como Windows-1252 e os acentos saem corrompidos. As linhas do relatório saem sempre ordenadas por nome de arquivo, para que duas execuções do mesmo lote possam ser comparadas lado a lado.

Assim como o manifesto de histórico, uma falha ao gravar o relatório (ex.: caminho sem permissão de escrita) nunca falha a organização, que já aconteceu de verdade — vira apenas um aviso no resultado.

## Desfazer uma organização

`organize-pdf` copia por padrão (não destrutivo), mas `--move` move de verdade — e quem roda um lote grande com uma regex recém-calibrada erra pelo menos uma vez. A partir desta versão, toda execução real (nunca uma simulação com `--dry-run`) grava um manifesto do que foi copiado ou movido, e `file-manager undo` reverte a partir dele.

**Só funciona para operações feitas a partir desta versão** — o manifesto não existia antes, então não há nada para o `undo` reverter de uma organização feita com uma versão anterior do `file-manager`.

```bash
file-manager undo --list              # lista as operações registradas
file-manager undo --last --dry-run    # mostra o que a última operação desfaria, sem tocar em nada
file-manager undo --last -y           # desfaz a última operação, sem pedir confirmação
file-manager undo --id 20260811-164530 -y   # desfaz uma operação específica
```

Sem `--id` nem `--last`, em terminal interativo o comando pergunta qual operação desfazer; sem terminal (ex: dentro de um script), falha com uma mensagem clara pedindo `--id` ou `--last`. Sem `-y`/`--yes`, pede confirmação mostrando quantos arquivos serão afetados. O mesmo fluxo também está disponível no menu principal, na opção "Desfazer uma organização" — que só aparece depois que pelo menos uma operação real foi registrada.

**Desfazer uma cópia é diferente de desfazer um movimento:**

- **Cópia** (`--move` não usado): desfazer **apaga** os arquivos criados no destino. O original em `--input` nunca é tocado — ele nunca tinha saído de lá.
- **Movimento** (`--move`): desfazer **devolve** os arquivos ao caminho de origem original.

**Regras de segurança, aplicadas sempre:**

- Nenhum arquivo fora do que foi registrado no manifesto é tocado.
- Antes de apagar ou mover um arquivo de volta, o `undo` verifica se ele ainda existe no destino e se o **tamanho** bate com o registrado na hora da organização. Tamanho diferente = o arquivo pode ter sido substituído ou editado depois → a entrada é **pulada**, nunca apagada. (A verificação é por tamanho, não por conteúdo — comparar o conteúdo inteiro de cada arquivo custaria caro demais num lote com centenas de PDFs.)
- Ao devolver um arquivo à origem, se já existir algo lá, a entrada é pulada — nunca sobrescreve.
- Diretórios que ficaram vazios no destino, depois do desfazer, são removidos; um diretório com qualquer outro arquivo dentro é preservado (nunca há remoção recursiva).
- Uma operação já desfeita não pode ser desfeita de novo sem `--force`.

| Flag | Descrição |
|------|-----------|
| `--id <id>` | ID da operação a desfazer |
| `--last` | Desfaz a operação registrada mais recente |
| `--dry-run` | Só mostra o que seria feito, sem tocar em nada |
| `-y, --yes` | Desfaz sem pedir confirmação |
| `--list` | Lista as operações registradas e sai |
| `--force` | Permite desfazer uma operação que já foi desfeita antes |

## Atualização

`file-manager update` consulta o último release publicado no GitHub, compara com a versão em execução e, se houver uma mais nova, pede confirmação antes de baixar e substituir o próprio executável.

| Flag | Descrição |
|------|-----------|
| `-y, --yes` | Atualiza sem pedir confirmação |
| `--check` | Só verifica se há versão nova, sem baixar nem substituir nada |

```bash
file-manager update           # verifica e pede confirmação
file-manager update -y        # atualiza sem perguntar
file-manager update --check   # só verifica
```

Se já estiver na última versão, o comando informa e sai sem erro. Antes de substituir o executável atual, o binário baixado é executado para validação — um download corrompido aborta a atualização sem tocar no executável em uso.

Além disso, o menu principal verifica em segundo plano (uma vez por sessão) se há uma versão mais nova. Na primeira abertura, o menu aguarda um instante (no máximo 1,5s) pelo resultado dessa verificação e, se houver versão mais nova, exibe um aviso com a progressão de versão e o comando para atualizar. Sem internet, ou se a verificação não chegar a tempo, o menu abre normalmente e nenhum aviso aparece.

## Perfis

Um perfil é um conjunto de configurações para uma ferramenta, salvo em um arquivo YAML reutilizável. Perfis aceleram a automação ao evitar redigitar os mesmos argumentos.

### Onde ficam os perfis

- **Windows:** `%AppData%\file-manager\profiles\<ferramenta>\<nome>.yaml`
- **Linux/macOS:** `~/.config/file-manager/profiles/<ferramenta>/<nome>.yaml`

### Criar um perfil

Use a tela interativa de perfis no menu principal. Selecione uma ferramenta, escolha "Perfis" e siga os prompts para criar um novo perfil. Suas respostas serão salvas no arquivo YAML.

### Usar um perfil

```bash
file-manager merge-pdf --profile meu-perfil
```

Se houver flags adicionais na linha de comando, elas sobrescrevem os valores do perfil.

### Exportar e importar um perfil

Quem calibra as regras de um perfil (ex: as expressões regulares de `organize-pdf`) e quem usa a ferramenta no dia a dia costumam ser pessoas diferentes, em máquinas diferentes — normalmente quem calibra é o dono do projeto, e quem usa é alguém sem familiaridade com regex. O fluxo é: calibrar uma vez numa máquina, exportar o perfil para um arquivo, enviar esse arquivo para a outra pessoa (e-mail, chat, o que for) e ela importar na própria máquina.

```bash
# Na máquina de quem calibrou:
file-manager profiles export --tool organize-pdf --name notas-fiscais --output notas-fiscais.yaml

# Depois de enviar notas-fiscais.yaml para a outra pessoa, na máquina dela:
file-manager profiles import --file notas-fiscais.yaml

# Para importar sob outro nome, ou por cima de um perfil já existente:
file-manager profiles import --file notas-fiscais.yaml --name notas-fiscais-2026 --force
```

O arquivo exportado é exatamente o mesmo envelope YAML usado internamente (`name`, `tool`, `created_at`, `updated_at`, `data`) — não existe um formato paralelo de exportação para manter sincronizado. Na importação, o `tool` declarado no arquivo precisa existir neste CLI e suportar perfis, e o conteúdo de `data` é validado contra as opções dessa ferramenta: um arquivo corrompido, editado à mão de forma inconsistente, ou exportado por uma versão incompatível do `file-manager`, falha na importação — não no meio de um lote de arquivos processado com um perfil ruim.

Outros comandos úteis:

```bash
file-manager profiles list                        # lista os perfis de todas as ferramentas
file-manager profiles list --tool organize-pdf     # lista só os perfis de uma ferramenta
file-manager profiles path                         # mostra o diretório onde os perfis ficam guardados
```

O mesmo fluxo (exportar/importar) também está disponível na tela interativa de perfis, como as opções "Exportar para arquivo" e "Importar de arquivo" no menu de ações de cada ferramenta.

## Documentação para IA

A ferramenta pode exportar sua própria documentação em dois formatos para uso em chatbots de IA:

### Formato `context`

```bash
file-manager docs export --format context --output docs.md
```

Gera um Markdown detalhado com a estrutura de toda a ferramenta, útil para colar em um chat de IA como contexto de um problema.

### Formato `skill`

```bash
file-manager docs export --format skill --output SKILL.md
```

Gera um arquivo `SKILL.md` com frontmatter YAML compatível com agentes de IA. Este arquivo pode ser instalado diretamente em um agente de IA para que a ferramenta fique acessível via slash-command (ex: `/file-manager`).

## Desenvolvimento

### Estrutura do projeto

```
cmd/file-manager/          Entrypoint do binário
cmd/scaffold/              Gerador de novas ferramentas
internal/app/              Registry de ferramentas + montagem do cobra
internal/ui/               Screen, Navigator, Clear cross-platform
internal/ui/filepicker/    Seleção interativa de arquivos/pastas
internal/ui/calibrate/     Calibração interativa de regex
internal/ui/profiles/      CRUD de perfis (genérico para todas as ferramentas)
internal/ui/undo/          Tela interativa de desfazer uma organização
internal/ui/docs/          Exportação de documentação
internal/tool/             Contrato Tool/Param/Doc
internal/config/           Gerenciamento de perfis YAML
internal/history/          Manifesto de operações reversíveis + lógica de desfazer
internal/pdfutil/          Núcleo: merge, split, organize, extração de texto
internal/regexcalib/       Sugestão de regex por exemplo
internal/tools/            Uma subpasta por ferramenta
internal/testcli/          Harness de teste ponta a ponta (terminal virtual, só Linux)
e2e/                       Cenários de teste ponta a ponta (rodam via "make e2e")
```

### Adicionar uma ferramenta nova

```bash
make new-tool NAME=minha-ferramenta
```

Veja [docs/CONTRIBUTING.md](docs/CONTRIBUTING.md) para o passo a passo completo.

### Compilar e testar

```bash
make build      # Compilar para a plataforma atual
make build-all  # Compilar para Linux e Windows
make test       # Executar testes
make e2e        # Executar testes ponta a ponta (terminal virtual, só Linux, mais lentos)
make lint       # Executar verificações de lint
make fmt        # Formatar código
make clean      # Remover artefatos de build
```

### Testes ponta a ponta

Além da suíte normal (`make test`), há testes que abrem o binário real dentro de um terminal virtual, enviam teclas como um usuário faria e verificam o que aparece na tela — cobrem navegação interativa (menu, seleção encadeada de pastas em `organize-pdf`) que `go test ./...` não exercita. Ficam em `e2e/` (mais o harness em `internal/testcli/`), rodam só em Linux e são mais lentos, por isso vivem fora do `make test` e têm alvo próprio: `make e2e`. Detalhes em [AGENTS.md](AGENTS.md#testes-ponta-a-ponta-e2e).

### Completação de shell (bash e zsh)

Isto é para quem usa o `file-manager` pelo terminal — se você abre o `.exe` com duplo clique no Windows e usa o menu interativo, pode pular esta seção: ela não muda nada no que você já usa.

O cobra (biblioteca de CLI usada por este projeto) gera o script de completação sob demanda, via `file-manager completion <shell>`. Ele não aparece na lista de comandos de `--help` (é o único comando deste CLI em inglês, então foi escondido para não confundir quem nunca vai usar shell nenhum), mas continua funcionando normalmente.

**Zsh:**

```bash
file-manager completion zsh > "${fpath[1]}/_file-manager"
```

Depois, abra um terminal novo (ou rode `autoload -U compinit; compinit`) para o zsh carregar o script. Se `${fpath[1]}` não for gravável, qualquer diretório já listado em `$fpath` serve.

**Bash** (requer o pacote `bash-completion` instalado):

```bash
file-manager completion bash > /etc/bash_completion.d/file-manager
```

Sem permissão de root, grave em `~/.local/share/bash-completion/completions/file-manager` (crie a pasta se não existir) e abra um terminal novo.

**O que a completação oferece:** além dos nomes dos comandos e flags (isso o cobra já dá de graça), o Tab completa **valores**: os IDs de operações que ainda podem ser desfeitas (`undo --id`), as ferramentas que suportam perfil (`profiles list/export --tool`), o arquivo de um perfil a importar ou de uma planilha filtrados por extensão (`profiles import --file`, `organize-pdf --csv`), as colunas da planilha já apontada em `--csv` (`organize-pdf --csv-levels`) e os valores aceitos por cada flag de enumeração (`--mode`, `--ocr`, `--ocr-lang`, `--report-format`, `--sort`). Uma consulta que falhe (ex: diretório de configuração inacessível, planilha ainda não criada) nunca aparece como erro no meio da linha de comando — só resulta em nenhuma sugestão.

## OCR

`split-pdf --mode regex` e `organize-pdf --level` classificam PDFs pelo conteúdo textual. Em um PDF digitalizado (imagem sem camada de texto), esse conteúdo não existe — por isso a ferramenta oferece OCR como alternativa: quando a página não tem texto embutido, a imagem da página é extraída e lida pelo [Tesseract](https://github.com/tesseract-ocr/tesseract).

Controlado pelas flags `--ocr` (`auto`, `always` ou `never`; default `auto`, que só aciona o OCR quando a página não tem texto) e `--ocr-lang` (idioma, default `por`).

**Requer o Tesseract instalado no sistema** — não é embutido no binário:

- **Windows:** instalador do [UB Mannheim](https://github.com/UB-Mannheim/tesseract/wiki), marcando o idioma Português durante a instalação.
- **Linux (Fedora/RHEL):** `sudo dnf install tesseract tesseract-langpack-por`
- **Linux (Debian/Ubuntu):** `sudo apt install tesseract-ocr tesseract-ocr-por`

Sem o Tesseract instalado, a ferramenta continua funcionando normalmente para PDFs com texto — apenas emite um aviso com a instrução de instalação acima e segue sem OCR.

**Custo:** o OCR é lento comparado à extração de texto nativa — aproximadamente 1 segundo por página. Para PDFs grandes, isso pode ser perceptível.

**Precisão:** o reconhecimento de caracteres não é perfeito (ex.: `ESCOLA` pode ser lido como `ESCO`, `0` pode ser confundido com `O`). Ao escrever regex para conteúdo que pode ter passado por OCR, prefira padrões tolerantes a esse tipo de erro em vez de casamentos exatos.

**Regex que atravessa linhas precisa de `(?s)`:** em Go, `.` não casa quebra de linha por padrão — e texto de OCR quebra linha o tempo todo. Se sua regex (em `--level` ou `--filename-regex`) precisa que `.` atravesse uma quebra de linha, use o prefixo `(?s)`. Exemplo real: `MATRÍCULA.*?(\d{6,})` não casava contra um PDF digitalizado; `(?s)MATRÍCULA.*?(\d{6,})` casou. Sem esse prefixo, a regex parece certa (funciona em testadores de regex genéricos, que costumam ligar esse modo por padrão) mas simplesmente não encontra nada dentro da ferramenta.

## Limitações conhecidas

Para PDFs digitalizados sem o Tesseract instalado (ou com `--ocr never`), os modos que dependem de regex sobre o conteúdo não funcionam. Use modos baseados em metadados ou estrutura do arquivo (ex: `split-pdf --mode page`) ou instale o Tesseract conforme a seção OCR acima.
