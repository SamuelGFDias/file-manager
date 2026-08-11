//go:build e2e && linux

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUndoListSurvivesCorruptedManifest é o cenário ponta a ponta pedido
// para o problema mais grave desta entrega: um manifesto de histórico
// ilegível (arquivo truncado por disco cheio, processo interrompido no
// momento errado, etc.) não pode derrubar "undo --list" inteiro — e com
// ele, o "undo" de todas as outras operações, inclusive as íntegras.
//
// Organiza duas vezes (dois manifestos válidos, em duas pastas de origem
// diferentes), corrompe um terceiro arquivo escrito diretamente no
// diretório de histórico isolado por XDG_CONFIG_HOME, e confirma que
// "undo --list" continua mostrando os dois manifestos válidos E exibe um
// aviso citando o arquivo corrompido — em vez de simplesmente falhar.
func TestUndoListSurvivesCorruptedManifest(t *testing.T) {
	configEnv := "XDG_CONFIG_HOME=" + t.TempDir()

	inputA := t.TempDir()
	outputA := t.TempDir()
	writeTestPDF(t, inputA, "nota-a.pdf", []string{"conteudo de teste a"})
	organizeOutA, _ := runCLI(t, inputA, []string{
		"organize-pdf", "--input", inputA, "--output", outputA,
	}, configEnv)
	t.Logf("organize-pdf (A):\n%s", organizeOutA)

	inputB := t.TempDir()
	outputB := t.TempDir()
	writeTestPDF(t, inputB, "nota-b.pdf", []string{"conteudo de teste b"})
	organizeOutB, _ := runCLI(t, inputB, []string{
		"organize-pdf", "--input", inputB, "--output", outputB,
	}, configEnv)
	t.Logf("organize-pdf (B):\n%s", organizeOutB)

	// O diretório de histórico fica em <XDG_CONFIG_HOME>/file-manager/history
	// — mesmo caminho que internal/history.Dir() resolve dentro do
	// processo. Aqui é construído diretamente porque o teste precisa
	// escrever um arquivo corrompido nele, sem passar pelo binário (que
	// nunca gravaria um manifesto inválido de propósito).
	xdgConfigHome := strings.TrimPrefix(configEnv, "XDG_CONFIG_HOME=")
	historyDir := filepath.Join(xdgConfigHome, "file-manager", "history")
	if _, err := os.Stat(historyDir); err != nil {
		t.Fatalf("diretório de histórico não existe após organizar (%q): %v", historyDir, err)
	}

	corruptedPath := filepath.Join(historyDir, "20261231-235959.yaml")
	if err := os.WriteFile(corruptedPath, []byte("isto: [nao é yaml válido\n\tlixo binário"), 0o644); err != nil {
		t.Fatalf("criar manifesto corrompido: %v", err)
	}

	listOut, _ := runCLI(t, inputA, []string{"undo", "--list"}, configEnv)
	t.Logf("undo --list (com manifesto corrompido no diretório):\n%s", listOut)

	if !strings.Contains(listOut, inputA) {
		t.Fatalf("undo --list não mostrou o manifesto válido de A (%q); saída:\n%s", inputA, listOut)
	}
	if !strings.Contains(listOut, inputB) {
		t.Fatalf("undo --list não mostrou o manifesto válido de B (%q); saída:\n%s", inputB, listOut)
	}
	if !strings.Contains(listOut, "20261231-235959.yaml") {
		t.Fatalf("undo --list não avisou sobre o manifesto corrompido; saída:\n%s", listOut)
	}

	// organize-pdf sem --move copia (o padrão): nota-b.pdf continua em
	// inputB e também aparece em outputB depois de organizar.
	if _, err := os.Stat(filepath.Join(outputB, "nota-b.pdf")); err != nil {
		t.Fatalf("esperava nota-b.pdf em outputB após organizar: %v", err)
	}

	// A prova final: "undo" continua funcionando de verdade apesar do
	// arquivo corrompido no meio do diretório — desfaz a operação mais
	// recente (B, uma cópia): apaga o que foi criado em outputB, sem
	// tocar no original em inputB.
	undoOut, _ := runCLI(t, inputB, []string{"undo", "--last", "-y"}, configEnv)
	t.Logf("undo --last -y (apesar do manifesto corrompido):\n%s", undoOut)

	if _, err := os.Stat(filepath.Join(outputB, "nota-b.pdf")); !os.IsNotExist(err) {
		t.Fatalf("esperava nota-b.pdf removido de outputB após undo, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(inputB, "nota-b.pdf")); err != nil {
		t.Fatalf("nota-b.pdf original em inputB não deveria ter sido tocado: %v", err)
	}
}
