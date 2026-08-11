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

// TestOCRPdfPastaSemPDFAvisaAntesDeSufixoEIdioma mira o defeito relatado
// (ver AGENTS.md, Decisão 19): o usuário escolheu "Escolher arquivos
// específicos", navegou até uma pasta sem PDF nenhum e — antes desta
// correção — nada impedia o avanço até seis perguntas depois (sufixo,
// idioma, sobrescrita, retomada, simulação), quando o programa finalmente
// falhava com "informe ao menos um arquivo ou pasta em --input". Este
// teste prova que o aviso de pasta sem PDF aparece NA HORA, antes de
// qualquer uma dessas perguntas — e que a lista (vazia) de seleção nunca
// chega a ser mostrada.
//
// Ao contrário de TestOCRPdfDryRunClassifiesEligibleAndSkipped, este
// cenário é sobre a TELA interativa (não sobre --dry-run via linha de
// comando) e não precisa do Tesseract de verdade: a validação de entradas
// acontece antes de qualquer checagem de motor de OCR (ver
// internal/tools/ocrpdf/command.go, ocrizeRaw), então roda com o ambiente
// isolado padrão (startBin), que deliberadamente tira o Tesseract do PATH.
func TestOCRPdfPastaSemPDFAvisaAntesDeSufixoEIdioma(t *testing.T) {
	emptyDir := t.TempDir() // deliberadamente sem nenhum PDF

	sess := startBin(t, emptyDir)
	defer sess.Close()

	sess.Expect("Tornar PDF pesquisável", defaultTimeout)
	sess.Send("Tornar PDF pesquisável")
	sess.Enter()

	sess.Expect("Como deseja adicionar entradas para processar?", defaultTimeout)
	sess.Enter() // "Escolher arquivos específicos" é a opção padrão (primeiro item)

	sess.Expect("Diretório atual: "+emptyDir, defaultTimeout)

	// A pasta está vazia (sem subpastas nem PDFs): as opções são só
	// ".. (voltar)", "[ Escolher arquivos desta pasta ]" e "[ Cancelar ]",
	// nessa ordem (ver filepicker.PickFiles) — "[ Escolher arquivos desta
	// pasta ]" é a segunda.
	sess.Down()
	sess.Enter()

	sess.Expect("não tem nenhum arquivo com a extensão .pdf", defaultTimeout)

	// A prova central do defeito corrigido: nenhuma pergunta seguinte
	// (sufixo, idioma) pode ter aparecido, e a lista de seleção múltipla
	// (que estaria vazia e não comunicaria nada) nunca deveria ter sido
	// mostrada — o aviso de pasta vazia intercepta ANTES do
	// survey.MultiSelect, não depois de uma seleção vazia confirmada.
	screen := sess.Screen()
	for _, naoEsperado := range []string{
		"Selecione os arquivos",
		"Sufixo do arquivo gerado",
		"Idioma do OCR",
	} {
		if strings.Contains(screen, naoEsperado) {
			t.Fatalf(
				"encontrou %q na tela ao escolher uma pasta sem PDFs — o aviso deveria bloquear o avanço "+
					"antes de qualquer pergunta seguinte.\n--- tela capturada ---\n%s",
				naoEsperado, screen,
			)
		}
	}

	// Encerra o cenário de forma limpa: o aviso não avança a etapa, então o
	// mesmo prompt de navegação está de volta na tela (o loop de
	// PickFiles continua na mesma pasta) — escolhe "[ Cancelar ]" (terceiro
	// item). As teclas enviadas aqui são absorvidas pelo pty mesmo que o
	// redesenho ainda não tenha terminado (o kernel enfileira a entrada até
	// o processo voltar a ler), então não é preciso um Expect extra entre
	// o aviso e este envio.
	sess.Down()
	sess.Down()
	sess.Enter()
}
