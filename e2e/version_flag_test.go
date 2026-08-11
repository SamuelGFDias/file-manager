//go:build e2e && linux

package e2e

import (
	"strings"
	"testing"
	"time"

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

// TestVersionFlagRedirectedProducesOnlyVersionLine é a prova de ponta a
// ponta (binário real, não uma chamada direta a cobra.Command.Execute())
// das duas armadilhas que o aviso de atualização disponível precisa
// resolver ao ser acrescentado a "--version"/"version" (ver AGENTS.md,
// Decisão 12): instantâneo e sem rede quando a saída não é um terminal.
// runCLI (undo_test.go) usa exec.Command comum — sem pty — então
// cmd.Stdout é um os.Pipe comum, não um tty: exatamente o que acontece com
// "file-manager --version > arquivo" ou "file-manager --version | cat".
// Sem terminal, printVersion (internal/app/root.go) nunca constrói um
// selfupdate.Checker, então não há requisição de rede — o binário deste
// teste roda numa suíte que pode não ter internet, e mesmo assim o teste
// passa rápido e determinístico.
func TestVersionFlagRedirectedProducesOnlyVersionLine(t *testing.T) {
	dir := t.TempDir()

	start := time.Now()
	stdout, stderr := runCLI(t, dir, []string{"--version"})
	elapsed := time.Since(start)

	want := app.Version{Version: "dev", Commit: "none", Date: "unknown"}.String() + "\n"
	if stdout != want {
		t.Errorf("stdout = %q, esperava exatamente %q (só a linha da versão, sem aviso)", stdout, want)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, esperava vazio — sem terminal não pode haver aviso de atualização", stderr)
	}

	// 2s é uma folga larga sobre o teto de 1s que printVersion respeita
	// (versionNoticeTimeout, em internal/app/root.go): o caminho
	// não-terminal nem chega a criar o Checker, então o tempo real
	// esperado é o de iniciar e encerrar o processo, não o de qualquer
	// espera de rede. Um valor alto o bastante evita flakiness em CI
	// carregado, mas ainda pega uma regressão que reintroduza uma consulta
	// síncrona no caminho redirecionado.
	if elapsed > 2*time.Second {
		t.Errorf("\"--version\" redirecionado levou %s, esperava resposta praticamente imediata (sem consulta de rede)", elapsed)
	}
}
