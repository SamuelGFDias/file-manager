//go:build e2e && linux

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOrganizeCSVBuildsHierarchyFromSpreadsheet é o cenário ponta a ponta
// pedido para a feature de organização guiada por planilha: monta
// exatamente a planilha do enunciado (com acentos), gera PDFs contendo as
// chaves de nota fiscal ("NOTA: 001" etc.) e roda "organize-pdf --csv
// --csv-key-regex" pela linha de comando (sem terminal interativo — tudo
// via flag, mesmo espírito de TestOrganizeReportGeneratesCSVWithExpectedLineCount
// em report_test.go), confirmando que a árvore
// Sao_Goncalo/Laranjal/001.pdf foi criada com acentos removidos e espaços
// virando "_".
func TestOrganizeCSVBuildsHierarchyFromSpreadsheet(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	csvPath := filepath.Join(t.TempDir(), "planilha.csv")
	csvContent := "NOTA,CIDADE,BAIRRO\n" +
		"001,São Gonçalo,Laranjal\n" +
		"003,Rio de Janeiro,Centro\n" +
		"005,Niterói,Fonseca\n"
	if err := os.WriteFile(csvPath, []byte(csvContent), 0o644); err != nil {
		t.Fatalf("criar planilha de teste: %v", err)
	}

	writeTestPDF(t, inputDir, "doc-001.pdf", []string{"NOTA: 001"})
	writeTestPDF(t, inputDir, "doc-003.pdf", []string{"NOTA: 003"})
	writeTestPDF(t, inputDir, "doc-005.pdf", []string{"NOTA: 005"})

	stdout, _ := runCLI(t, inputDir, []string{
		"organize-pdf",
		"--input", inputDir,
		"--output", outputDir,
		"--csv", csvPath,
		"--csv-key-regex", `NOTA:\s*(\d+)`,
	})

	if !strings.Contains(stdout, "3 de 3") {
		t.Errorf("saída do comando não confirma os 3 arquivos organizados:\n%s", stdout)
	}

	for _, want := range []string{
		filepath.Join(outputDir, "Sao_Goncalo", "Laranjal", "001.pdf"),
		filepath.Join(outputDir, "Rio_de_Janeiro", "Centro", "003.pdf"),
		filepath.Join(outputDir, "Niteroi", "Fonseca", "005.pdf"),
	} {
		if _, err := os.Stat(want); err != nil {
			t.Errorf("arquivo esperado não foi criado em %s: %v", want, err)
		}
	}

	// Nenhuma pasta com acento deveria ter sido criada — a normalização é
	// o ponto central desta feature (rede compartilhada / Windows-Linux).
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatalf("ler outputDir: %v", err)
	}
	for _, e := range entries {
		if strings.ContainsAny(e.Name(), "ãçéí") {
			t.Errorf("nome de pasta %q ainda contém acento; deveria ter sido normalizado", e.Name())
		}
	}
}

// TestOrganizeCSVUnknownKeyGoesToUnclassifiedWithReason confirma o caso
// mais comum na prática apontado no enunciado: uma chave que o regex
// encontra no PDF mas que não existe na planilha não interrompe o lote — o
// arquivo cai em --unclassified-dir e o relatório (--report) cita a chave
// encontrada no motivo, para conferir na planilha.
func TestOrganizeCSVUnknownKeyGoesToUnclassifiedWithReason(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	reportPath := filepath.Join(t.TempDir(), "relatorio.csv")

	csvPath := filepath.Join(t.TempDir(), "planilha.csv")
	csvContent := "NOTA,CIDADE,BAIRRO\n001,São Gonçalo,Laranjal\n"
	if err := os.WriteFile(csvPath, []byte(csvContent), 0o644); err != nil {
		t.Fatalf("criar planilha de teste: %v", err)
	}

	writeTestPDF(t, inputDir, "doc-001.pdf", []string{"NOTA: 001"})
	// Chave 999 não está na planilha.
	writeTestPDF(t, inputDir, "doc-999.pdf", []string{"NOTA: 999"})

	stdout, _ := runCLI(t, inputDir, []string{
		"organize-pdf",
		"--input", inputDir,
		"--output", outputDir,
		"--csv", csvPath,
		"--csv-key-regex", `NOTA:\s*(\d+)`,
		"--report", reportPath,
	})

	if !strings.Contains(stdout, "1 de 2") {
		t.Errorf("saída do comando não confirma 1 de 2 arquivos organizados:\n%s", stdout)
	}
	if !strings.Contains(stdout, "999") {
		t.Errorf("saída do comando deveria citar a chave 999 não encontrada na planilha:\n%s", stdout)
	}

	unclassifiedPath := filepath.Join(outputDir, "sem-classificacao", "doc-999.pdf")
	if _, err := os.Stat(unclassifiedPath); err != nil {
		t.Errorf("arquivo com chave desconhecida deveria estar em sem-classificacao: %v", err)
	}

	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("relatório não foi criado em %s: %v", reportPath, err)
	}
	content := string(data)
	if !strings.Contains(content, "doc-999.pdf") || !strings.Contains(content, "999") {
		t.Errorf("relatório deveria conter a linha do arquivo não classificado citando a chave 999:\n%s", content)
	}
}

// TestOrganizeCSVSemicolonSeparatorDetected prova a detecção automática do
// separador ponto e vírgula — o que o Excel em português salva por
// padrão.
func TestOrganizeCSVSemicolonSeparatorDetected(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	csvPath := filepath.Join(t.TempDir(), "planilha.csv")
	csvContent := "NOTA;CIDADE;BAIRRO\n001;São Gonçalo;Laranjal\n"
	if err := os.WriteFile(csvPath, []byte(csvContent), 0o644); err != nil {
		t.Fatalf("criar planilha de teste: %v", err)
	}

	writeTestPDF(t, inputDir, "doc-001.pdf", []string{"NOTA: 001"})

	stdout, _ := runCLI(t, inputDir, []string{
		"organize-pdf",
		"--input", inputDir,
		"--output", outputDir,
		"--csv", csvPath,
		"--csv-key-regex", `NOTA:\s*(\d+)`,
	})

	if !strings.Contains(stdout, "1 de 1") {
		t.Errorf("saída do comando não confirma o arquivo organizado (separador ; deveria ter sido detectado):\n%s", stdout)
	}

	want := filepath.Join(outputDir, "Sao_Goncalo", "Laranjal", "001.pdf")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("arquivo esperado não foi criado em %s: %v", want, err)
	}
}

// TestOrganizeCSVWithLevelIsRejected confirma a validação de linha de
// comando: --csv e --level juntos são incompatíveis e o comando deve
// falhar antes de processar qualquer arquivo (saindo com código de erro
// diferente de zero).
func TestOrganizeCSVWithLevelIsRejected(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	csvPath := filepath.Join(t.TempDir(), "planilha.csv")
	if err := os.WriteFile(csvPath, []byte("NOTA,CIDADE\n001,Acme\n"), 0o644); err != nil {
		t.Fatalf("criar planilha de teste: %v", err)
	}

	cmdArgs := []string{
		"organize-pdf",
		"--input", inputDir,
		"--output", outputDir,
		"--csv", csvPath,
		"--csv-key-regex", `NOTA:\s*(\d+)`,
		"--level", `fornecedor=FORNECEDOR:\s*(\w+)`,
	}

	stdout, stderr, err := runCLIExpectingError(t, inputDir, cmdArgs)
	if err == nil {
		t.Fatalf("comando deveria ter falhado com --csv e --level juntos.\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "--csv") || !strings.Contains(combined, "--level") {
		t.Errorf("mensagem de erro deveria mencionar --csv e --level:\n%s", combined)
	}
}
