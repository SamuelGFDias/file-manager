//go:build e2e && linux

package e2e

import (
	"strings"
	"testing"

	"github.com/SamuelGFDias/file-manager/internal/app"
)

// TestVersionFlagMatchesSubcommand é a prova de ponta a ponta (binário real
// compilado a partir deste checkout, não uma chamada direta a
// cobra.Command.Execute()) de que "--version" e o atalho "-v" imprimem
// exatamente o mesmo texto que o subcomando "version" — a mesma garantia já
// coberta em internal/app/version_flag_test.go, aqui contra o processo de
// verdade. runCLI (undo_test.go) já falha o teste (t.Fatalf) se qualquer
// uma das três chamadas sair com código diferente de zero, então este
// cenário também cobre a exigência de código de saída 0 para as três
// formas.
func TestVersionFlagMatchesSubcommand(t *testing.T) {
	dir := t.TempDir()

	flagOut, _ := runCLI(t, dir, []string{"--version"})
	shortOut, _ := runCLI(t, dir, []string{"-v"})
	subOut, _ := runCLI(t, dir, []string{"version"})

	if flagOut != subOut {
		t.Fatalf("saída de \"--version\" (%q) difere da saída do subcomando \"version\" (%q)", flagOut, subOut)
	}
	if shortOut != subOut {
		t.Fatalf("saída de \"-v\" (%q) difere da saída do subcomando \"version\" (%q)", shortOut, subOut)
	}

	// binPath (TestMain, main_test.go) é compilado sem -ldflags, então
	// version/commit/date ficam nos defaults declarados em
	// cmd/file-manager/main.go ("dev"/"none"/"unknown").
	want := app.Version{Version: "dev", Commit: "none", Date: "unknown"}.String() + "\n"
	if flagOut != want {
		t.Errorf("saída de \"--version\" = %q, esperava %q", flagOut, want)
	}
}

// TestHelpListsVersionFlagInPortuguese confirma, contra o binário real, que
// "--help" lista a flag "-v, --version" com a descrição traduzida — não o
// "version for file-manager" em inglês que o cobra geraria por padrão.
func TestHelpListsVersionFlagInPortuguese(t *testing.T) {
	dir := t.TempDir()

	out, _ := runCLI(t, dir, []string{"--help"})

	if !strings.Contains(out, "-v, --version") {
		t.Errorf("saída de --help não lista a flag \"-v, --version\":\n%s", out)
	}
	if !strings.Contains(out, "mostra a versão do binário") {
		t.Errorf("saída de --help não contém a descrição em português da flag --version:\n%s", out)
	}
}
