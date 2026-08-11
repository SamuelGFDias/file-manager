package app

import (
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestTools(t *testing.T) {
	tools := Tools()

	if len(tools) != 3 {
		t.Fatalf("Tools() devolveu %d ferramentas, esperava 3", len(tools))
	}

	wantIDs := []string{"merge-pdf", "split-pdf", "organize-pdf"}
	gotIDs := make([]string, len(tools))
	for i, tl := range tools {
		gotIDs[i] = tl.Meta().ID
	}

	for i, want := range wantIDs {
		if gotIDs[i] != want {
			t.Errorf("Tools()[%d].Meta().ID = %q, esperava %q", i, gotIDs[i], want)
		}
	}
}

func TestToolsUniqueIDs(t *testing.T) {
	tools := Tools()

	seen := make(map[string]bool, len(tools))
	for _, tl := range tools {
		id := tl.Meta().ID
		if seen[id] {
			t.Fatalf("ID de ferramenta duplicado: %q", id)
		}
		seen[id] = true
	}
}

func TestNewRootCommandUse(t *testing.T) {
	cmd := NewRootCommand(Version{Version: "0.1.0", Commit: "abc1234", Date: "2026-08-11T12:00:00Z"})

	if cmd.Use != "file-manager" {
		t.Errorf("cmd.Use = %q, esperava %q", cmd.Use, "file-manager")
	}
}

func TestNewRootCommandHasSubcommands(t *testing.T) {
	cmd := NewRootCommand(Version{Version: "0.1.0", Commit: "abc1234", Date: "2026-08-11T12:00:00Z"})

	names := make(map[string]bool)
	for _, c := range cmd.Commands() {
		names[c.Name()] = true
	}

	wantNames := []string{"merge-pdf", "split-pdf", "organize-pdf", "docs", "version", "update"}
	for _, want := range wantNames {
		if !names[want] {
			t.Errorf("subcomando %q não encontrado; subcomandos presentes: %v", want, names)
		}
	}
}

func TestVersionString(t *testing.T) {
	v := Version{Version: "0.1.0", Commit: "abc1234", Date: "2026-08-11T12:00:00Z"}
	s := v.String()

	for _, part := range []string{v.Version, v.Commit, v.Date} {
		if !strings.Contains(s, part) {
			t.Errorf("Version.String() = %q, esperava que contivesse %q", s, part)
		}
	}
}

func TestDocsExportCommandFlags(t *testing.T) {
	cmd := NewRootCommand(Version{Version: "0.1.0", Commit: "abc1234", Date: "2026-08-11T12:00:00Z"})

	var found bool
	for _, c := range cmd.Commands() {
		if c.Name() != "docs" {
			continue
		}
		for _, sub := range c.Commands() {
			if sub.Name() != "export" {
				continue
			}
			found = true
			if sub.Flags().Lookup("format") == nil {
				t.Errorf("subcomando docs export não tem a flag --format")
			}
			if sub.Flags().Lookup("output") == nil {
				t.Errorf("subcomando docs export não tem a flag --output")
			}
		}
	}

	if !found {
		t.Fatalf("subcomando \"docs export\" não encontrado")
	}
}

func TestUpdateCommandFlags(t *testing.T) {
	cmd := NewRootCommand(Version{Version: "0.1.0", Commit: "abc1234", Date: "2026-08-11T12:00:00Z"})

	var update *cobra.Command
	for _, c := range cmd.Commands() {
		if c.Name() == "update" {
			update = c
			break
		}
	}

	if update == nil {
		t.Fatalf("subcomando \"update\" não encontrado")
	}

	yesFlag := update.Flags().Lookup("yes")
	if yesFlag == nil {
		t.Fatalf("subcomando update não tem a flag --yes")
	}
	if yesFlag.Shorthand != "y" {
		t.Errorf("flag --yes tem shorthand %q, esperava \"y\"", yesFlag.Shorthand)
	}

	if update.Flags().Lookup("check") == nil {
		t.Errorf("subcomando update não tem a flag --check")
	}
}

func TestProfilesCommandHasSubcommands(t *testing.T) {
	cmd := NewRootCommand(Version{Version: "0.1.0", Commit: "abc1234", Date: "2026-08-11T12:00:00Z"})

	var profilesCmd *cobra.Command
	for _, c := range cmd.Commands() {
		if c.Name() == "profiles" {
			profilesCmd = c
			break
		}
	}
	if profilesCmd == nil {
		t.Fatalf("subcomando \"profiles\" não encontrado")
	}

	names := make(map[string]bool)
	for _, c := range profilesCmd.Commands() {
		names[c.Name()] = true
	}

	wantNames := []string{"list", "export", "import", "path"}
	for _, want := range wantNames {
		if !names[want] {
			t.Errorf("subcomando \"profiles %s\" não encontrado; presentes: %v", want, names)
		}
	}
}

func TestProfilesListCommandFlags(t *testing.T) {
	cmd := NewRootCommand(Version{Version: "0.1.0", Commit: "abc1234", Date: "2026-08-11T12:00:00Z"})

	listCmd := findSubcommand(t, cmd, "profiles", "list")
	if listCmd.Flags().Lookup("tool") == nil {
		t.Errorf("subcomando profiles list não tem a flag --tool")
	}
}

func TestProfilesExportCommandFlags(t *testing.T) {
	cmd := NewRootCommand(Version{Version: "0.1.0", Commit: "abc1234", Date: "2026-08-11T12:00:00Z"})

	exportCmd := findSubcommand(t, cmd, "profiles", "export")

	for _, name := range []string{"tool", "name", "output"} {
		flag := exportCmd.Flags().Lookup(name)
		if flag == nil {
			t.Errorf("subcomando profiles export não tem a flag --%s", name)
			continue
		}
	}

	for _, name := range []string{"tool", "name", "output"} {
		if !isRequiredFlag(exportCmd, name) {
			t.Errorf("flag --%s de profiles export deveria ser obrigatória", name)
		}
	}
}

func TestProfilesImportCommandFlags(t *testing.T) {
	cmd := NewRootCommand(Version{Version: "0.1.0", Commit: "abc1234", Date: "2026-08-11T12:00:00Z"})

	importCmd := findSubcommand(t, cmd, "profiles", "import")

	if importCmd.Flags().Lookup("file") == nil {
		t.Errorf("subcomando profiles import não tem a flag --file")
	}
	if importCmd.Flags().Lookup("name") == nil {
		t.Errorf("subcomando profiles import não tem a flag --name")
	}
	if importCmd.Flags().Lookup("force") == nil {
		t.Errorf("subcomando profiles import não tem a flag --force")
	}

	if !isRequiredFlag(importCmd, "file") {
		t.Errorf("flag --file de profiles import deveria ser obrigatória")
	}
}

// TestProfilesImportCommandDecodeErrorIsUserFriendly reproduz o caso
// relatado pelo coordenador: um arquivo com "tool: organize-pdf" e
// "data.levels" sendo uma string em vez de lista. Antes da correção, a
// mensagem de erro devolvida ao usuário era o erro cru do decodificador de
// YAML ("cannot unmarshal !!str ... into []organizepdf.LevelSpec"), em
// inglês e citando um tipo interno do Go — incompreensível para quem
// recebeu o arquivo por e-mail e só quer saber se pode importar ou não. A
// mensagem final precisa nomear a ferramenta e o arquivo, explicar as
// causas comuns em português, sugerir pedir um novo arquivo a quem enviou,
// e ainda assim preservar o erro original (encapsulado com %w) para quem
// for depurar.
func TestProfilesImportCommandDecodeErrorIsUserFriendly(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "perfil-incompativel.yaml")
	content := "name: teste\ntool: organize-pdf\ndata:\n  levels: \"isso deveria ser uma lista\"\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("falha ao preparar arquivo de teste: %v", err)
	}

	cmd := NewRootCommand(Version{Version: "0.1.0", Commit: "abc1234", Date: "2026-08-11T12:00:00Z"})
	cmd.SetArgs([]string{"profiles", "import", "--file", file})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("esperava erro ao importar perfil com \"data\" incompatível")
	}

	msg := err.Error()

	// A mensagem principal precisa ser compreensível para quem não é
	// desenvolvedor: cita a ferramenta e o arquivo, explica as causas
	// comuns, e sugere o que fazer.
	for _, want := range []string{
		"organize-pdf",
		file,
		"corrompido",
		"editado à mão",
		"versão diferente",
		"exportá-lo novamente",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("mensagem de erro deveria conter %q; got: %s", want, msg)
		}
	}

	// O erro cru do decodificador não pode ter desaparecido: precisa
	// continuar acessível para quem for depurar, encapsulado com %w.
	if errors.Unwrap(err) == nil {
		t.Fatalf("erro deveria encapsular o erro original do decodificador via %%w; got: %v", err)
	}
	if !strings.Contains(msg, "detalhe técnico") {
		t.Errorf("mensagem deveria expor o erro original numa segunda linha \"detalhe técnico:\"; got: %s", msg)
	}
}

func TestProfilesPathCommandExists(t *testing.T) {
	cmd := NewRootCommand(Version{Version: "0.1.0", Commit: "abc1234", Date: "2026-08-11T12:00:00Z"})

	_ = findSubcommand(t, cmd, "profiles", "path")
}

// findSubcommand navega de root até o subcomando identificado pela cadeia
// de nomes em path, falhando o teste se algum elo não existir.
func findSubcommand(t *testing.T, root *cobra.Command, path ...string) *cobra.Command {
	t.Helper()

	current := root
	for _, name := range path {
		var next *cobra.Command
		for _, c := range current.Commands() {
			if c.Name() == name {
				next = c
				break
			}
		}
		if next == nil {
			t.Fatalf("subcomando %q não encontrado em %q", name, current.Name())
		}
		current = next
	}
	return current
}

// isRequiredFlag verifica se uma flag foi marcada como obrigatória via
// cmd.MarkFlagRequired, olhando a annotation que o cobra usa internamente.
func isRequiredFlag(cmd *cobra.Command, name string) bool {
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		return false
	}
	annotations, ok := flag.Annotations[cobra.BashCompOneRequiredFlag]
	return ok && len(annotations) > 0 && annotations[0] == "true"
}

func TestRootCommandHelp(t *testing.T) {
	cmd := NewRootCommand(Version{Version: "0.1.0", Commit: "abc1234", Date: "2026-08-11T12:00:00Z"})
	cmd.SetArgs([]string{"--help"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("cmd.Execute() com --help devolveu erro inesperado: %v", err)
	}
}
