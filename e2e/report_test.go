//go:build e2e && linux

package e2e

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOrganizeReportGeneratesCSVWithExpectedLineCount é o cenário ponta a
// ponta pedido para a feature de relatório: roda "organize-pdf" com
// --report pela linha de comando (sem terminal interativo, tudo via
// flag — mesmo espírito de runCLI em undo_test.go) e confirma que o CSV
// gerado existe, começa com o BOM UTF-8 e tem exatamente uma linha de
// cabeçalho mais uma linha por PDF do lote (2 classificados + 1 não
// classificado = 3), na ordem alfabética por nome de arquivo.
func TestOrganizeReportGeneratesCSVWithExpectedLineCount(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	reportPath := filepath.Join(t.TempDir(), "relatorio.csv")

	writeTestPDF(t, inputDir, "nota1.pdf", []string{"FORNECEDOR: ACME conteudo de teste"})
	writeTestPDF(t, inputDir, "nota2.pdf", []string{"FORNECEDOR: GLOBEX conteudo de teste"})
	writeTestPDF(t, inputDir, "nota3.pdf", []string{"nenhum fornecedor reconhecivel aqui"})

	stdout, _ := runCLI(t, inputDir, []string{
		"organize-pdf",
		"--input", inputDir,
		"--output", outputDir,
		"--level", `fornecedor=FORNECEDOR:\s*(\w+)`,
		"--report", reportPath,
	})

	if !strings.Contains(stdout, "relatório gravado em") {
		t.Errorf("saída do comando não confirma o caminho do relatório gerado:\n%s", stdout)
	}

	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("relatório não foi criado em %s: %v", reportPath, err)
	}

	content := string(data)
	if !strings.HasPrefix(content, "\ufeff") {
		t.Fatalf("relatório não começa com o BOM UTF-8; primeiros bytes: %q", content[:minInt(20, len(content))])
	}
	content = strings.TrimPrefix(content, "\ufeff")

	scanner := bufio.NewScanner(strings.NewReader(content))
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("erro ao ler linhas do relatório: %v", err)
	}

	// 1 cabeçalho + 3 arquivos (2 classificados + 1 não classificado).
	const wantLines = 4
	if len(lines) != wantLines {
		t.Fatalf("relatório tem %d linhas, esperava %d.\nconteúdo:\n%s", len(lines), wantLines, content)
	}

	wantHeader := "arquivo,origem,destino,classificado,motivo"
	if lines[0] != wantHeader {
		t.Errorf("cabeçalho = %q, want %q", lines[0], wantHeader)
	}

	// Ordem alfabética por nome de arquivo: nota1, nota2, nota3.
	if !strings.HasPrefix(lines[1], "nota1.pdf,") || !strings.Contains(lines[1], ",sim,") {
		t.Errorf("linha 1 inesperada: %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "nota2.pdf,") || !strings.Contains(lines[2], ",sim,") {
		t.Errorf("linha 2 inesperada: %q", lines[2])
	}
	if !strings.HasPrefix(lines[3], "nota3.pdf,") || !strings.Contains(lines[3], ",nao,") {
		t.Errorf("linha 3 inesperada (esperava não classificado com motivo preenchido): %q", lines[3])
	}
	// CSV escapa aspas duplicando-as ("" dentro de um campo entre aspas),
	// então checa o motivo por partes em vez do texto exato entre aspas.
	if !strings.Contains(lines[3], "fornecedor") || !strings.Contains(lines[3], "não encontrado") {
		t.Errorf("linha 3 deveria conter o motivo da não classificação: %q", lines[3])
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
