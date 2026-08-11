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
internal/ui/docs/          Exportação de documentação
internal/tool/             Contrato Tool/Param/Doc
internal/config/           Gerenciamento de perfis YAML
internal/pdfutil/          Núcleo: merge, split, organize, extração de texto
internal/regexcalib/       Sugestão de regex por exemplo
internal/tools/            Uma subpasta por ferramenta
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
make lint       # Executar verificações de lint
make fmt        # Formatar código
make clean      # Remover artefatos de build
```

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
