package app

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/SamuelGFDias/file-manager/internal/history"
)

func testVersion() Version {
	return Version{Version: "0.1.0", Commit: "abc1234", Date: "2026-08-11T12:00:00Z"}
}

// TestCompletionCommandHiddenButFunctional prova as duas metades da
// decisão: o comando "completion" some da lista de comandos disponíveis
// (o que aparece em --help), mas continua existindo e funcionando de
// verdade para quem o invoca diretamente. HiddenDefaultCmd faz exatamente
// isso; DisableDefaultCmd (que removeria a funcionalidade) não é usado.
func TestCompletionCommandHiddenButFunctional(t *testing.T) {
	root := NewRootCommand(testVersion())

	// InitDefaultCompletionCmd só acrescenta o comando "completion" na
	// hora de ExecuteC (chamado a partir de cmd.Execute()) — fora de uma
	// execução completa, precisa ser chamado explicitamente, como o cobra
	// faz internamente.
	root.InitDefaultCompletionCmd()

	found, _, err := root.Find([]string{"completion"})
	if err != nil {
		t.Fatalf("root.Find([\"completion\"]) devolveu erro: %v", err)
	}
	if found == nil || found.Name() != "completion" {
		t.Fatalf("root.Find([\"completion\"]) não encontrou o comando; got %v", found)
	}
	if !found.Hidden {
		t.Errorf("comando completion deveria estar Hidden, não está")
	}

	for _, c := range root.Commands() {
		if c.Name() == "completion" && c.IsAvailableCommand() {
			t.Errorf("comando completion não deveria passar em IsAvailableCommand() (deveria estar escondido de --help)")
		}
	}
}

// TestHelpCommandIsPortuguese garante que o comando "help" (substituído em
// NewRootCommand via SetHelpCommand) usa textos em português, não o
// "Help about any command" padrão do cobra.
func TestHelpCommandIsPortuguese(t *testing.T) {
	root := NewRootCommand(testVersion())
	// InitDefaultHelpCmd só acrescenta "help" como filho de root na hora de
	// ExecuteC — fora de uma execução completa, precisa ser chamado
	// explicitamente.
	root.InitDefaultHelpCmd()

	helpCmd := findSubcommand(t, root, "help")

	if strings.Contains(strings.ToLower(helpCmd.Short), "help about") {
		t.Errorf("Short do comando help ainda está em inglês: %q", helpCmd.Short)
	}
	if !strings.Contains(helpCmd.Short, "ajuda") && !strings.Contains(helpCmd.Short, "Ajuda") {
		t.Errorf("Short do comando help não parece estar em português: %q", helpCmd.Short)
	}
}

// TestHelpFlagIsPortuguese garante que o texto da flag --help (herdada via
// PersistentFlags do comando raiz) está em português, tanto no comando
// raiz quanto em um subcomando qualquer (a herança é o que garante que o
// cobra não crie sua própria versão em inglês por comando).
func TestHelpFlagIsPortuguese(t *testing.T) {
	root := NewRootCommand(testVersion())

	rootHelp := root.PersistentFlags().Lookup("help")
	if rootHelp == nil {
		t.Fatal("comando raiz não tem flag --help registrada")
	}
	if strings.Contains(strings.ToLower(rootHelp.Usage), "help for") {
		t.Errorf("Usage de --help no comando raiz ainda está em inglês: %q", rootHelp.Usage)
	}

	mergeCmd := findSubcommand(t, root, "merge-pdf")
	mergeCmd.InitDefaultHelpFlag()
	mergeHelp := mergeCmd.Flags().Lookup("help")
	if mergeHelp == nil {
		t.Fatal("merge-pdf não tem flag --help (nem própria, nem herdada)")
	}
	if strings.Contains(strings.ToLower(mergeHelp.Usage), "help for") {
		t.Errorf("Usage de --help em merge-pdf ainda está em inglês: %q", mergeHelp.Usage)
	}
}

// --- undo --id ---

// TestUndoIDCompletionEmptyHistory prova que, sem nenhuma operação
// registrada, a completação de --id devolve lista vazia sem erro.
func TestUndoIDCompletionEmptyHistory(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cmd := findSubcommand(t, NewRootCommand(testVersion()), "undo")

	fn, ok := cmd.GetFlagCompletionFunc("id")
	if !ok {
		t.Fatal("nenhuma função de completação registrada para --id")
	}

	got, directive := fn(cmd, nil, "")
	if len(got) != 0 {
		t.Errorf("completação de --id sem histórico = %v, esperava lista vazia", got)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
}

// TestUndoIDCompletionOnlyPending prova que a completação de --id só
// oferece manifestos ainda não desfeitos: sugerir um ID já revertido leva
// o usuário a um erro evitável ("já foi desfeita").
func TestUndoIDCompletionOnlyPending(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if _, _, err := history.Save(history.Manifest{
		Tool:      "organize-pdf",
		CreatedAt: time.Now(),
		InputDir:  "/tmp/origem-pendente",
		OutputDir: "/tmp/destino-pendente",
		Action:    history.ActionCopy,
	}); err != nil {
		t.Fatalf("history.Save(pendente): %v", err)
	}
	if _, _, err := history.Save(history.Manifest{
		Tool:      "organize-pdf",
		CreatedAt: time.Now().Add(-time.Hour),
		InputDir:  "/tmp/origem-desfeita",
		OutputDir: "/tmp/destino-desfeito",
		Action:    history.ActionCopy,
	}); err != nil {
		t.Fatalf("history.Save(desfeita): %v", err)
	}

	// history.Save gera o ID internamente (recebe Manifest por valor, então
	// o ID gerado não volta para o struct do chamador) — recarrega a lista
	// para achar o ID de verdade do manifesto que este teste quer marcar
	// como já desfeito.
	headers, _, err := history.List()
	if err != nil {
		t.Fatalf("history.List(): %v", err)
	}
	for _, h := range headers {
		if h.InputDir == "/tmp/origem-desfeita" {
			if err := history.MarkUndone(h.ID, time.Now()); err != nil {
				t.Fatalf("history.MarkUndone(%q): %v", h.ID, err)
			}
		}
	}

	cmd := findSubcommand(t, NewRootCommand(testVersion()), "undo")
	fn, ok := cmd.GetFlagCompletionFunc("id")
	if !ok {
		t.Fatal("nenhuma função de completação registrada para --id")
	}

	got, directive := fn(cmd, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
	if len(got) != 1 {
		t.Fatalf("completação de --id = %v, esperava exatamente 1 entrada (só o manifesto pendente)", got)
	}
	if !strings.Contains(got[0], "/tmp/origem-pendente") {
		t.Errorf("completação de --id[0] = %q, esperava conter a pasta de origem do manifesto pendente", got[0])
	}
	if strings.Contains(got[0], "origem-desfeita") {
		t.Errorf("completação de --id não deveria oferecer o manifesto já desfeito: %v", got)
	}
}

// TestUndoIDCompletionMissingConfigDirNeverFails garante que, mesmo com o
// diretório de configuração apontando para algo inacessível/inexistente, a
// completação de --id nunca propaga erro — devolve lista vazia.
func TestUndoIDCompletionMissingConfigDirNeverFails(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/caminho/que/nao/existe/de/verdade")

	cmd := findSubcommand(t, NewRootCommand(testVersion()), "undo")
	fn, ok := cmd.GetFlagCompletionFunc("id")
	if !ok {
		t.Fatal("nenhuma função de completação registrada para --id")
	}

	got, directive := fn(cmd, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
	_ = got // não deve haver panic nem erro; conteúdo exato não importa aqui
}

// TestUndoIDCompletionIgnoresCorruptedManifestWarnings garante que a
// completação de --id continua funcionando com um manifesto corrompido no
// diretório de histórico — oferecendo o(s) manifesto(s) íntegro(s)
// normalmente — e, ao contrário de "undo --list" (ver
// TestPrintUndoListWarnsAboutCorruptedManifest, em undo_test.go), NUNCA
// imprime o aviso: um Tab não pode cuspir aviso no meio da linha de
// comando que o usuário está digitando.
func TestUndoIDCompletionIgnoresCorruptedManifestWarnings(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if _, _, err := history.Save(history.Manifest{
		Tool:      "organize-pdf",
		CreatedAt: time.Now(),
		InputDir:  "/tmp/origem-integra",
		OutputDir: "/tmp/destino-integro",
		Action:    history.ActionCopy,
	}); err != nil {
		t.Fatalf("history.Save(): %v", err)
	}

	dir, err := history.Dir()
	if err != nil {
		t.Fatalf("history.Dir(): %v", err)
	}
	corrupted := dir + "/20261231-235959.yaml"
	if err := os.WriteFile(corrupted, []byte("isto: [nao é yaml válido"), 0o644); err != nil {
		t.Fatalf("criar manifesto corrompido: %v", err)
	}

	cmd := findSubcommand(t, NewRootCommand(testVersion()), "undo")
	fn, ok := cmd.GetFlagCompletionFunc("id")
	if !ok {
		t.Fatal("nenhuma função de completação registrada para --id")
	}

	var got []string
	var directive cobra.ShellCompDirective
	out := captureStdout(t, func() {
		got, directive = fn(cmd, nil, "")
	})

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
	if len(got) != 1 || !strings.Contains(got[0], "/tmp/origem-integra") {
		t.Fatalf("completação de --id = %v, esperava exatamente o manifesto íntegro", got)
	}
	if out != "" {
		t.Fatalf("completação de --id não deveria imprimir nada (nem aviso do manifesto corrompido); saída:\n%s", out)
	}
}

// --- profiles --tool ---

// TestProfileToolCompletionOnlySupportingTools garante que a completação
// de --tool (profiles list / profiles export) só oferece IDs de
// ferramentas que implementam suporte a perfis salvos (Profile() != nil).
func TestProfileToolCompletionOnlySupportingTools(t *testing.T) {
	root := NewRootCommand(testVersion())

	for _, path := range [][]string{
		{"profiles", "list"},
		{"profiles", "export"},
	} {
		cmd := findSubcommand(t, root, path...)
		fn, ok := cmd.GetFlagCompletionFunc("tool")
		if !ok {
			t.Fatalf("%v: nenhuma função de completação registrada para --tool", path)
		}

		got, directive := fn(cmd, nil, "")
		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("%v: directive = %v, want ShellCompDirectiveNoFileComp", path, directive)
		}

		wantIDs := map[string]bool{}
		for _, tl := range Tools() {
			if tl.Profile() != nil {
				wantIDs[tl.Meta().ID] = true
			}
		}

		if len(got) != len(wantIDs) {
			t.Fatalf("%v: completação de --tool = %v, esperava %d entradas (uma por ferramenta com suporte a perfil)", path, got, len(wantIDs))
		}
		for _, entry := range got {
			id := strings.SplitN(entry, "\t", 2)[0]
			if !wantIDs[id] {
				t.Errorf("%v: completação de --tool ofereceu %q, que não suporta perfis salvos", path, id)
			}
		}
	}
}

// --- profiles import --file ---

// TestProfileImportFileCompletionFiltersYAML garante que --file (profiles
// import) delega a completação de arquivo ao cobra, filtrando pelas
// extensões de um perfil exportado (yaml/yml), em vez de listar candidatos
// manualmente.
func TestProfileImportFileCompletionFiltersYAML(t *testing.T) {
	cmd := findSubcommand(t, NewRootCommand(testVersion()), "profiles", "import")

	fn, ok := cmd.GetFlagCompletionFunc("file")
	if !ok {
		t.Fatal("nenhuma função de completação registrada para --file")
	}

	got, directive := fn(cmd, nil, "")
	if directive != cobra.ShellCompDirectiveFilterFileExt {
		t.Errorf("directive = %v, want ShellCompDirectiveFilterFileExt", directive)
	}

	want := map[string]bool{"yaml": true, "yml": true}
	if len(got) != len(want) {
		t.Fatalf("completação de --file = %v, esperava extensões %v", got, want)
	}
	for _, ext := range got {
		if !want[ext] {
			t.Errorf("completação de --file ofereceu extensão inesperada %q", ext)
		}
	}
}

// TestProfileToolCompletionNeverPanicsOnEmptyConfigDir garante que a
// completação de --tool nunca falha nem entra em pânico mesmo com o
// diretório de configuração isolado e vazio (não há I/O nessa função —
// Tools() é só construção em memória — mas o teste documenta e protege
// essa garantia).
func TestProfileToolCompletionNeverPanicsOnEmptyConfigDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cmd := findSubcommand(t, NewRootCommand(testVersion()), "profiles", "list")
	fn, ok := cmd.GetFlagCompletionFunc("tool")
	if !ok {
		t.Fatal("nenhuma função de completação registrada para --tool")
	}

	got, directive := fn(cmd, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
	if len(got) == 0 {
		t.Errorf("completação de --tool devolveu lista vazia; esperava as ferramentas com suporte a perfil")
	}
}
