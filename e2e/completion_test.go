//go:build e2e && linux

package e2e

import (
	"strings"
	"testing"
)

// TestHelpDoesNotListCompletionCommand prova a metade "esconder" da
// decisão: o cobra acrescenta sozinho um subcomando "completion" (a única
// peça do programa em inglês, num CLI todo em português) — HiddenDefaultCmd
// tira ele da lista de comandos disponíveis mostrada por --help, mas não
// desativa a funcionalidade (ver TestCompletionCommandStillWorks abaixo).
// Roda como processo simples (sem pty): --help não é interativo.
func TestHelpDoesNotListCompletionCommand(t *testing.T) {
	out, _ := runCLI(t, "", []string{"--help"})

	if strings.Contains(out, "completion") {
		t.Fatalf("saída de --help ainda menciona \"completion\" (deveria estar escondido):\n%s", out)
	}

	// Confirma que a ajuda tem conteúdo de verdade (nenhum teste que só
	// checa AUSÊNCIA de uma palavra pega uma saída vazia por acidente).
	for _, want := range []string{"merge-pdf", "split-pdf", "organize-pdf"} {
		if !strings.Contains(out, want) {
			t.Errorf("saída de --help não contém %q:\n%s", want, out)
		}
	}
}

// TestCompletionCommandStillWorks confirma a metade "continua funcionando"
// da decisão: mesmo escondido de --help, "file-manager completion zsh"
// produz o script de completação normalmente.
func TestCompletionCommandStillWorks(t *testing.T) {
	out, _ := runCLI(t, "", []string{"completion", "zsh"})

	if strings.TrimSpace(out) == "" {
		t.Fatal("file-manager completion zsh não produziu saída")
	}
}

// TestSplitModeCompletionListsFixedValues exercita o comando interno que o
// cobra usa para completar valores de flag ("__complete"), o mesmo que um
// shell de verdade invoca ao apertar Tab, contra uma flag de enumeração
// simples (sem I/O nenhum): split-pdf --mode.
func TestSplitModeCompletionListsFixedValues(t *testing.T) {
	out, _ := runCLI(t, "", []string{"__complete", "split-pdf", "--mode", ""})

	for _, want := range []string{"page", "range", "regex"} {
		if !strings.Contains(out, want) {
			t.Errorf("__complete split-pdf --mode '' não contém %q:\n%s", want, out)
		}
	}
}

// TestUndoIDCompletionListsRegisteredOperations exercita "__complete undo
// --id ”" (o mesmo comando interno acima) contra o caso que de fato
// justifica esta feature: completar um valor DINÂMICO, que só existe
// porque uma operação real foi registrada. Organiza uma pasta de verdade
// primeiro (gerando um manifesto de histórico), depois confirma que a
// completação lista a pasta de origem dessa operação.
func TestUndoIDCompletionListsRegisteredOperations(t *testing.T) {
	configEnv := "XDG_CONFIG_HOME=" + t.TempDir()

	inputDir := t.TempDir()
	outputDir := t.TempDir()
	writeTestPDF(t, inputDir, "nota.pdf", []string{"NF: 42"})

	organizeOut, _ := runCLI(t, inputDir, []string{
		"organize-pdf",
		"--input", inputDir,
		"--output", outputDir,
		"--filename-regex", `NF:\s*(\d+)`,
	}, configEnv)
	t.Logf("organize-pdf:\n%s", organizeOut)

	completeOut, _ := runCLI(t, inputDir, []string{"__complete", "undo", "--id", ""}, configEnv)
	t.Logf("__complete undo --id '':\n%s", completeOut)

	if !strings.Contains(completeOut, inputDir) {
		t.Fatalf("__complete undo --id '' não menciona a pasta de origem organizada (%q):\n%s", inputDir, completeOut)
	}
}
