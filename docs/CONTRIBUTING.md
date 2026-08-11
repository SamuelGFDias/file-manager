# Guia de Contribuição

Bem-vindo! Este documento descreve como contribuir para o projeto file-manager, especialmente como adicionar uma ferramenta nova.

## Adicionando uma Ferramenta Nova

### Passo 1: Gerar o esqueleto

```bash
make new-tool NAME=minha-ferramenta
```

O scaffold gerará uma estrutura em `internal/tools/minha-ferramenta/` com os arquivos iniciais.

### Passo 2: Implementar a lógica de domínio

A camada de lógica pura (sem I/O ou interfaces) deve ficar em seu próprio pacote dentro de `internal/<dominio>util/`.

**Por quê?** Essa separação permite testar a lógica isoladamente, sem mockar telas ou arquivos. Exemplos:

- `internal/mergepdfutil/` → contém `Merge()` função pura
- `internal/splitpdfutil/` → contém `Split()` função pura
- `internal/organizepdfutil/` → contém `Organize()` função pura
- `internal/regexcalib/` → contém `SuggestFromExample()` função pura

A tela da ferramenta (em `internal/tools/<ferramenta>/screen.go`) é apenas cola de I/O que chama essa lógica.

### Passo 3: Preencher os arquivos da ferramenta

#### `options.go`

Define os tipos que armazenam as opções da ferramenta durante e depois da interação. Exemplo:

```go
type Options struct {
    Input    string
    Output   string
    MaxDepth int
    Sort     string
    Overwrite bool
}
```

#### `params()` em `command.go`

Declare cada parâmetro UMA ÚNICA VEZ como `tool.Param`. Exemplo:

```go
func (t *MyTool) params() []tool.Param {
    return []tool.Param{
        {
            Name:     "input",
            Short:    "i",
            Type:     tool.String,
            Usage:    "Arquivo ou pasta de entrada",
            Required: true,
        },
        {
            Name:     "output",
            Short:    "o",
            Type:     tool.String,
            Usage:    "Caminho do arquivo de saída",
            Required: true,
        },
    }
}
```

**Regra de ouro:** Cada `tool.Param` é a fonte única de verdade. Dele saem:
1. A flag do Cobra (automaticamente)
2. A pergunta interativa no `screen.go` (referenciando `p.Name`)
3. A documentação exportada em `Doc().Flags` (automaticamente)

**Nunca** duplique essas três coisas. Se a documentação diferir da flag real ou da pergunta, é sinal de que a declaração não foi atualizada.

#### `screen.go`

Implementa `ExecuteInteractive()` com prompts para cada parâmetro. Esta camada é **cola de I/O**, não lógica de negócio.

```go
func (t *MyTool) ExecuteInteractive(ctx context.Context, nav *ui.Navigator) error {
    // Usar survey para perguntar Input
    // Usar survey para perguntar Output
    // Validar as respostas
    // Chamar t.Execute() com as opções preenchidas
    return nil
}
```

Não teste linha a linha o conteúdo de `screen.go` — é deliberadamente não testado porque é só I/O.

#### `tool.go`

Implemente o contrato `tool.Tool`. Especialmente importante:

```go
func (t *MyTool) Doc() tool.Doc {
    return tool.Doc{
        Name:        "my-tool",
        Title:       "Minha Ferramenta",
        Description: "Descrição breve do que faz",
        // Flags são preenchidas automaticamente de params()
    }
}
```

Existe um teste (`internal/app/registry_test.go`) que valida que `Doc().Flags` corresponde exatamente às flags reais do Cobra. Isso trava a divergência.

### Passo 4: Registrar a ferramenta

Edite `internal/app/registry.go` e adicione sua ferramenta à lista de ferramentas disponíveis:

```go
func NewRegistry() *Registry {
    return &Registry{
        tools: map[string]tool.Tool{
            // ... outras ferramentas
            "my-tool": &mytoolutool.MyTool{},
        },
    }
}
```

### Passo 5: Escrever testes

Testes para lógica pura (funções de domínio) devem usar **testes de tabela**:

```go
func TestMerge(t *testing.T) {
    tests := []struct {
        name       string
        input      []string
        maxDepth   int
        wantErr    bool
        wantErrMsg string
    }{
        {
            name:     "merge two files",
            input:    []string{"a.pdf", "b.pdf"},
            maxDepth: 1,
            wantErr:  false,
        },
        {
            name:     "invalid path",
            input:    []string{"/nonexistent"},
            maxDepth: 1,
            wantErr:  true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := Merge(tt.input, tt.maxDepth)
            if (err != nil) != tt.wantErr {
                t.Fatalf("got error %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

Evite testes frágeis de integração ou testes de mocks complexos. Teste a lógica pura; I/O (arquivo, HTTP, etc.) pode ser testado com `testdata/` ou skipped.

### Passo 6: Compilar, testar e verificar

```bash
make test
make lint
make build
```

Se houver erros de lint, execute `make fmt` para auto-formatar.

### Passo 7: Atualizar a documentação

Atualize:
1. **`README.md`:** Adicione uma seção com a tabela de flags e 2 exemplos reais de comando.
2. **`CHANGELOG.md`:** Adicione uma entrada na seção `[Não publicado]` descrevendo a ferramenta nova.

## Arquitetura e Princípios

### Camadas

1. **Lógica pura** (`internal/<dominio>util/`): Funções sem I/O, testáveis com testes de tabela.
2. **Interface** (`internal/tools/<ferramenta>/`): Tela, command, tool. Cola de I/O e orquestração.
3. **App** (`internal/app/registry.go`): Registry de ferramentas e montagem do Cobra.

### Declaração Única de Parâmetros

Cada parâmetro é declarado uma vez em `params()`. Disso saem:

```
tool.Param → Cobra flag + Survey prompt + Doc().Flags
```

Se você vê duplicação de "input", "output", etc. em múltiplos lugares, é um code smell. Refatore para usar o contrato `tool.Param`.

### Teste de Docstring vs. Realidade

Em `internal/app/registry_test.go` existe um teste que compara as flags declaradas em `Doc().Flags` com as flags reais registradas no Cobra. Isso previne que alguém atualize a documentação mas esqueça de atualizar a flag real (ou vice-versa).

Mantenha esse teste passando sempre.

## Política de Versionamento

Siga a política descrita em [CHANGELOG.md](../CHANGELOG.md):

- **MAJOR:** Mudança incompatível em flags ou formato YAML.
- **MINOR:** Nova ferramenta, novo modo, nova flag (compatível).
- **PATCH:** Correção de bug, melhoria interna.

## Deprecação de Flags

Para remover uma flag no futuro:

1. No MINOR anterior à remoção, marque-a como deprecated usando Cobra:
   ```go
   cmd.Flags().StringVar(...) // flag antiga
   cmd.Flags().MarkDeprecated("nome-flag", "use --novo-nome em vez disso")
   ```

2. Aguarde **pelo menos um MINOR** com a flag deprecated.

3. Só então remova em uma versão MAJOR.

Isso garante que automações de usuários antigas tenham tempo de se adaptar.

## Checklist para Contribuições

- [ ] Fiz o scaffold com `make new-tool`
- [ ] Separei lógica pura em `internal/<dominio>util/`
- [ ] Preenchi `params()`, `screen.go`, `tool.go`
- [ ] Registrei em `internal/app/registry.go`
- [ ] Escrevi testes de tabela para a lógica pura
- [ ] `make test` passa
- [ ] `make lint` passa (ou rodei `make fmt`)
- [ ] `make build` gera o binário
- [ ] Atualizei `README.md` com exemplos
- [ ] Atualizei `CHANGELOG.md`

## Dúvidas?

Se tiver dúvidas sobre a arquitetura, regras de teste ou política de versionamento, veja:
- Este arquivo (CONTRIBUTING.md)
- [CHANGELOG.md](../CHANGELOG.md) — política de versionamento
- Código das ferramentas existentes (`internal/tools/mergepdf/`, etc.)
