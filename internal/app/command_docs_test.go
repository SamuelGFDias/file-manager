package app

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/SamuelGFDias/file-manager/internal/commanddocs"
)

// zeroDefaultEquivalentsAux espelha zeroDefaultEquivalents de
// consistency_test.go (não reutilizada de lá de propósito: este arquivo
// NÃO deve depender de consistency_test.go, que é o único arquivo deste
// pacote que a tarefa proíbe editar — mantê-los desacoplados evita que uma
// mudança neste arquivo tenha qualquer motivo para tocar naquele).
var zeroDefaultEquivalentsAux = map[string]bool{
	"":      true,
	"false": true,
	"0":     true,
	"[]":    true,
}

// TestRootCommandsAreAllDocumented é o teste que fecha, de vez, a lacuna
// que motivou esta mudança: percorre TODOS os subcomandos reais do comando
// raiz (o mesmo *cobra.Command que o binário executa de verdade, via
// NewRootCommand) e falha se algum comando "folha" (sem subcomandos
// próprios — é nesse nível que as flags de verdade vivem) não estiver
// documentado nem como ferramenta do registry (app.Tools()) nem como
// comando auxiliar (commanddocs.CommandDocs()).
//
// Antes desta correção, "undo", "profiles" (e seus 4 subcomandos),
// "update", "version" e "docs export" existiam como comandos reais e
// funcionais, mas nunca apareciam na documentação exportável — porque
// docs.Render só percorre app.Tools(), e nenhum desses é uma ferramenta do
// registry. Este teste garante que um comando novo, adicionado no futuro a
// NewRootCommand sem entrar em nenhuma das duas listas, QUEBRA a suíte até
// ser documentado — o único jeito de essa lacuna não voltar a acontecer em
// silêncio.
func TestRootCommandsAreAllDocumented(t *testing.T) {
	root := NewRootCommand(testVersion())

	// InitDefaultHelpCmd/InitDefaultCompletionCmd só acrescentam "help" e
	// "completion" à árvore de comandos na hora de uma execução completa
	// (ExecuteC) — chamados aqui explicitamente (mesmo padrão de
	// completion_test.go) para que este teste veja a árvore de comandos
	// exatamente como um usuário real veria em "--help", com ou sem essa
	// inicialização lazy já ter acontecido antes.
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()

	toolIDs := make(map[string]bool)
	for _, tl := range Tools() {
		toolIDs[tl.Meta().ID] = true
	}

	auxIDs := make(map[string]bool)
	for _, d := range commanddocs.CommandDocs() {
		auxIDs[d.ID] = true
	}

	var walk func(cmd *cobra.Command, path []string)
	walk = func(cmd *cobra.Command, path []string) {
		for _, child := range cmd.Commands() {
			name := child.Name()

			// "help" e "completion" são criados pelo próprio cobra
			// (Command.InitDefaultHelpCmd / InitDefaultCompletionCmd), não
			// são comandos deste CLI, e não têm Doc própria em lugar
			// nenhum do projeto (nem como ferramenta, nem como comando
			// auxiliar) — ignorados de propósito, não por omissão.
			if name == "help" || name == "completion" {
				continue
			}

			childPath := append(append([]string{}, path...), name)

			if len(child.Commands()) > 0 {
				// Comando "grupo" (profiles, docs): não tem flags
				// próprias, só existe para organizar subcomandos — quem
				// documenta é o subcomando folha, onde as flags reais
				// vivem. Recursa sem exigir Doc própria do grupo.
				walk(child, childPath)
				continue
			}

			id := strings.Join(childPath, " ")

			if len(childPath) == 1 && toolIDs[name] {
				continue // documentado como ferramenta do registry
			}

			if !auxIDs[id] {
				t.Errorf(
					"comando %q (folha real de NewRootCommand) não está documentado nem como "+
						"ferramenta (app.Tools()) nem como comando auxiliar "+
						"(commanddocs.CommandDocs() — esperava uma entrada com ID %q)",
					id, id,
				)
			}
		}
	}

	walk(root, nil)
}

// TestCommandDocsFlagsMatchCobra é, para os comandos auxiliares, o
// equivalente do que TestToolsConsistency (consistency_test.go) já garante
// para as ferramentas: toda flag declarada em commanddocs.CommandDocs()
// precisa existir de fato no comando cobra correspondente, e toda flag
// real do comando precisa estar documentada — nos dois sentidos, para que
// nem uma flag fantasma nem uma flag não documentada passem despercebidas.
func TestCommandDocsFlagsMatchCobra(t *testing.T) {
	root := NewRootCommand(testVersion())

	for _, d := range commanddocs.CommandDocs() {
		d := d
		t.Run(d.ID, func(t *testing.T) {
			cmd := findSubcommand(t, root, strings.Fields(d.ID)...)
			flags := cmd.Flags()

			docNames := make(map[string]bool, len(d.Flags))
			for _, fd := range d.Flags {
				docNames[fd.Name] = true

				f := flags.Lookup(fd.Name)
				if f == nil {
					t.Errorf("comando %q: Doc().Flags declara a flag %q, mas ela não existe em Command().Flags()",
						d.ID, fd.Name)
					continue
				}

				if fd.Default == "" {
					if !zeroDefaultEquivalentsAux[f.DefValue] {
						t.Errorf("comando %q: flag %q não declara Default, mas o default real da flag é %q (não é um zero-value)",
							d.ID, fd.Name, f.DefValue)
					}
				} else if fd.Default != f.DefValue {
					t.Errorf("comando %q: flag %q declara Default %q, mas o default real (DefValue) é %q",
						d.ID, fd.Name, fd.Default, f.DefValue)
				}

				if fd.Shorthand != f.Shorthand {
					t.Errorf("comando %q: flag %q declara Shorthand %q, mas o shorthand real é %q",
						d.ID, fd.Name, fd.Shorthand, f.Shorthand)
				}
			}

			flags.VisitAll(func(f *pflag.Flag) {
				if !docNames[f.Name] {
					t.Errorf("comando %q: a flag %q existe em Command().Flags(), mas não está documentada em Doc().Flags",
						d.ID, f.Name)
				}
			})
		})
	}
}

// TestCommandDocsHaveEssentialFields garante que nenhuma entrada de
// commanddocs.CommandDocs() tem campos essenciais vazios — o mesmo
// requisito que TestToolsConsistency já impõe às ferramentas (item 7 do
// seu comentário), para que a documentação dos comandos auxiliares não
// fique tão completa quanto vazia por dentro.
func TestCommandDocsHaveEssentialFields(t *testing.T) {
	for _, d := range commanddocs.CommandDocs() {
		if strings.TrimSpace(d.ID) == "" {
			t.Errorf("comanddocs: uma entrada tem ID vazio (Title: %q)", d.Title)
		}
		if strings.TrimSpace(d.Title) == "" {
			t.Errorf("comando %q: Title está vazio", d.ID)
		}
		if strings.TrimSpace(d.Summary) == "" {
			t.Errorf("comando %q: Summary está vazio", d.ID)
		}
		if strings.TrimSpace(d.Description) == "" {
			t.Errorf("comando %q: Description está vazia", d.ID)
		}
		if len(d.WhenToUse) == 0 {
			t.Errorf("comando %q: WhenToUse não tem nenhuma entrada", d.ID)
		}
		if len(d.Examples) < 2 {
			t.Errorf("comando %q: Examples tem %d entrada(s), esperava ao menos 2", d.ID, len(d.Examples))
		}

		// Todo exemplo precisa citar "file-manager" de verdade em algum
		// lugar do comando — sem exigir um prefixo fixo (ao contrário de
		// TestToolsConsistency para ferramentas): comandos auxiliares têm
		// formas de invocação legítimas que não começam com
		// "file-manager <id>" ao pé da letra, como "file-manager
		// --version" (documentado em "version") ou um exemplo que embute o
		// comando dentro de uma substituição de shell (documentado em
		// "profiles path").
		for i, ex := range d.Examples {
			if !strings.Contains(ex.Command, "file-manager") {
				t.Errorf("comando %q: Examples[%d] (%q) tem comando %q, que não parece invocar o file-manager",
					d.ID, i, ex.Title, ex.Command)
			}
		}
	}
}
