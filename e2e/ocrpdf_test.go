//go:build e2e && linux

package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestOCRPdfDryRunClassifiesEligibleAndSkipped é o cenário ponta a ponta
// pedido para ocr-pdf: roda "ocr-pdf --dry-run" pela linha de comando
// (sem terminal interativo, mesmo espírito de runCLI em undo_test.go)
// sobre uma pasta com um PDF puro scan (elegível) e um PDF já com texto
// embutido (recusado, por economia — ver DecideEligibility em
// internal/pdfutil/ocrize.go), e confirma que o resumo da simulação
// reporta exatamente 1 processado e 1 pulado, com o motivo do pulado
// citado.
//
// Ao contrário dos demais cenários e2e deste pacote, este NÃO pode usar o
// ambiente isolado padrão (isolatedEnv/runCLI): eles deliberadamente
// removem o Tesseract do PATH para manter os testes que não são sobre OCR
// determinísticos (ver helpers_test.go). ocr-pdf EXIGE o Tesseract mesmo
// em --dry-run (é o próprio propósito da ferramenta — ver AGENTS.md), então
// este teste localiza o tesseract de verdade da máquina via
// exec.LookPath (usando o PATH real do processo de teste, não o
// sandboxed) e o aponta via TESSERACT_PATH; sem tesseract instalado no
// ambiente, pula com um motivo claro em vez de falhar.
func TestOCRPdfDryRunClassifiesEligibleAndSkipped(t *testing.T) {
	tesseractPath, err := exec.LookPath("tesseract")
	if err != nil {
		t.Skip("tesseract não está instalado neste ambiente; pulando cenário e2e de ocr-pdf (a ferramenta exige o Tesseract mesmo em --dry-run)")
	}

	inputDir := t.TempDir()
	writeImagePDF(t, inputDir, "scan.pdf")                               // puro scan: elegível
	writeTestPDF(t, inputDir, "com-texto.pdf", []string{"ja tem texto"}) // já pesquisável: recusado por economia

	cmd := exec.Command(binPath, "ocr-pdf", "--input", inputDir, "--dry-run")
	cmd.Dir = inputDir
	// Ambiente próprio (não isolatedEnv): precisa do tesseract de verdade
	// no PATH/TESSERACT_PATH, mas ainda com XDG_CONFIG_HOME isolado para
	// não tocar na configuração real do usuário rodando o teste.
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"TESSERACT_PATH=" + tesseractPath,
		"XDG_CONFIG_HOME=" + t.TempDir(),
	}

	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		t.Fatalf(
			"ocr-pdf --dry-run falhou: %v\nstdout:\n%s\nstderr:\n%s",
			err, out.String(), errBuf.String(),
		)
	}

	stdout := out.String()

	if !strings.Contains(stdout, "[simulação]") {
		t.Errorf("saída não indica modo simulação:\n%s", stdout)
	}
	if !strings.Contains(stdout, "1 de 2 arquivos processados, 1 pulados") {
		t.Errorf("resumo da simulação não bate com o esperado (1 elegível, 1 pulado):\n%s", stdout)
	}
	if !strings.Contains(stdout, "com-texto.pdf") || !strings.Contains(stdout, "já tem texto embutido") {
		t.Errorf("saída não cita o motivo do arquivo pulado (com-texto.pdf, já tem texto):\n%s", stdout)
	}
	if !strings.Contains(stdout, "scan.pdf") {
		t.Errorf("saída não cita o arquivo elegível (scan.pdf):\n%s", stdout)
	}

	// --dry-run não pode ter gerado nenhum arquivo novo na pasta.
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		t.Fatalf("ler pasta de entrada: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("--dry-run criou arquivo(s) além dos dois originais: %d entradas em %s", len(entries), inputDir)
	}
}
