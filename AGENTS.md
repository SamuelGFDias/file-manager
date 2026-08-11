# AGENTS.md — file-manager

Um conjunto de ferramentas de linha de comando para gerenciar, manipular e organizar arquivos PDF com precisão e automação. Desenvolvido em Go puro (`CGO_ENABLED=0`), roda em Windows e Linux como um único binário, sem dependências externas.

## Visão Geral

- **Módulo:** `github.com/SamuelGFDias/file-manager`
- **Go:** 1.26.5
- **Binário:** `file-manager` (entrypoint: `cmd/file-manager`)
- **Ferramentas:** `merge-pdf`, `split-pdf`, `organize-pdf`
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
internal/ui/mainmenu/             Menu principal
internal/ui/docs/                 Exportação de documentação (context e skill)
internal/tool/                    Contrato Tool/Param/Doc
internal/config/                  Gerenciamento de perfis YAML (paths, validação, I/O)
internal/pdfutil/                 Núcleo: merge, split, organize, extração de texto (com fallback OCR)
internal/ocr/                     Wrapper do executável externo tesseract (não é binding CGO)
internal/regexcalib/              Sugestão de regex a partir de valor de exemplo
internal/tools/                   Uma subpasta por ferramenta (mergepdf/, splitpdf/, organizepdf/)
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
| `make lint` | Executar `go vet` e `golangci-lint` (se instalado) |
| `make fmt` | Formatar código com gofmt |
| `make new-tool NAME=x` | Gerar esqueleto de ferramenta nova |
| `make docs` | Exportar documentação em `dist/` (formatos context e skill) |
| `make clean` | Remover artefatos de build (`dist/`) |

**Build com flags:** O Makefile injeta versão, commit e data via `-ldflags` (variáveis `main.version`, `main.commit`, `main.date`).

**CGO:** Todas as compilações usam `CGO_ENABLED=0` (Go puro, sem C).

## Processo de Release

Push de uma tag `v*` dispara `.github/workflows/release.yml`: o workflow roda `go test ./...` como gate, compila os binários de Linux e Windows com `CGO_ENABLED=0` e publica o release com os dois artefatos anexados.

**Lançar uma versão nova:**
```bash
git tag -a vX.Y.Z -m "..."
git push origin vX.Y.Z
```
Nada mais é necessário — o workflow cuida de teste, build e publicação.

A versão reportada por `file-manager version` vem da tag, injetada via `-ldflags` (o mesmo mecanismo de `main.version` descrito acima) — por isso a tag é a fonte da verdade da versão, não o código.

O job precisa de `permissions: contents: write` no workflow; sem isso a publicação do release falha com 403.

As notas geradas automaticamente (`generate_release_notes: true`) são só o changelog de commits desde a tag anterior. Para notas descritivas em português voltadas ao usuário final, editar depois com `gh release edit <tag> --notes-file <arquivo>`.

## Exportação de Documentação

A ferramenta consegue exportar sua própria documentação para uso em chatbots:

```bash
file-manager docs export --format context --output docs.md
file-manager docs export --format skill --output SKILL.md
```

- **Formato `context`:** Markdown detalhado com estrutura completa (útil para colar em chat de IA)
- **Formato `skill`:** Markdown com frontmatter YAML compatível com agentes de IA (`.md` pode ser instalado como skill)

Os metadados exportados (flags, exemplos, etc.) são gerados **automaticamente** a partir de `tool.Param`, reforçando a regra: uma única fonte de verdade.

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

## Exploração Adicional

- **docs/CONTRIBUTING.md** — Guia detalhado de contribuição com exemplos
- **README.md** — Uso de ferramentas, exemplos de linha de comando
- **internal/app/consistency_test.go** — Teste `TestToolsConsistency` que valida documentação vs. flags reais e integridade da registração de ferramentas
- **internal/tools/mergepdf/**, **splitpdf/**, **organizepdf/** — Exemplos de implementação completa

