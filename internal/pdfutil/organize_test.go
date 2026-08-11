package pdfutil

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestResolveDestinationAllLevelsMatch(t *testing.T) {
	levels := []Level{
		{Label: "ano", Regex: regexp.MustCompile(`Ano: (\d{4})`)},
		{Label: "empresa", Regex: regexp.MustCompile(`Empresa: (\w+)`)},
	}
	filenameRegex := regexp.MustCompile(`Nota: (\d+)`)

	text := "Empresa: Acme\nAno: 2023\nNota: 42"

	relPath, unmatched := ResolveDestination(text, levels, filenameRegex)
	if unmatched != nil {
		t.Fatalf("não esperava falha de classificação: %+v", unmatched)
	}
	want := filepath.Join("2023", "Acme", "42.pdf")
	if relPath != want {
		t.Fatalf("relPath = %q, esperava %q", relPath, want)
	}
}

func TestResolveDestinationMiddleLevelFails(t *testing.T) {
	levels := []Level{
		{Label: "ano", Regex: regexp.MustCompile(`Ano: (\d{4})`)},
		{Label: "empresa", Regex: regexp.MustCompile(`Empresa: (\w+)`)},
		{Label: "setor", Regex: regexp.MustCompile(`Setor: (\w+)`)},
	}

	text := "Ano: 2023\nSetor: Vendas" // falta "Empresa:"

	relPath, unmatched := ResolveDestination(text, levels, nil)
	if unmatched == nil {
		t.Fatal("esperava falha de classificação no nível 'empresa'")
	}
	if unmatched.Level != "empresa" {
		t.Fatalf("Level = %q, esperava %q", unmatched.Level, "empresa")
	}
	if relPath != "" {
		t.Fatalf("relPath deveria ser vazio em caso de falha, obteve %q", relPath)
	}
}

func TestResolveDestinationFilenameRegexFails(t *testing.T) {
	levels := []Level{
		{Label: "ano", Regex: regexp.MustCompile(`Ano: (\d{4})`)},
	}
	filenameRegex := regexp.MustCompile(`Nota: (\d+)`)

	text := "Ano: 2023" // sem "Nota:"

	_, unmatched := ResolveDestination(text, levels, filenameRegex)
	if unmatched == nil {
		t.Fatal("esperava falha de classificação no filename")
	}
	if unmatched.Level != "filename" {
		t.Fatalf("Level = %q, esperava %q", unmatched.Level, "filename")
	}
}

func TestResolveDestinationRenameOnlyMode(t *testing.T) {
	filenameRegex := regexp.MustCompile(`Nota: (\d+)`)
	text := "Nota: 999"

	relPath, unmatched := ResolveDestination(text, nil, filenameRegex)
	if unmatched != nil {
		t.Fatalf("não esperava falha: %+v", unmatched)
	}
	if relPath != "999.pdf" {
		t.Fatalf("relPath = %q, esperava %q (modo somente renomear)", relPath, "999.pdf")
	}
}

func TestResolveDestinationNilFilenameRegexOnlyFolders(t *testing.T) {
	levels := []Level{
		{Label: "ano", Regex: regexp.MustCompile(`Ano: (\d{4})`)},
	}
	text := "Ano: 2023"

	relPath, unmatched := ResolveDestination(text, levels, nil)
	if unmatched != nil {
		t.Fatalf("não esperava falha: %+v", unmatched)
	}
	if relPath != "2023" {
		t.Fatalf("relPath = %q, esperava só a pasta %q", relPath, "2023")
	}
}

func TestResolveDestinationEmptyTextUnclassified(t *testing.T) {
	levels := []Level{
		{Label: "ano", Regex: regexp.MustCompile(`Ano: (\d{4})`)},
	}

	relPath, unmatched := ResolveDestination("", levels, nil)
	if unmatched == nil {
		t.Fatal("esperava falha de classificação para texto vazio")
	}
	if relPath != "" {
		t.Fatalf("relPath deveria ser vazio, obteve %q", relPath)
	}
}

func TestResolveDestinationSanitizesTraversalInCapture(t *testing.T) {
	levels := []Level{
		{Label: "empresa", Regex: regexp.MustCompile(`Empresa: (.+)`)},
	}
	filenameRegex := regexp.MustCompile(`Nota: (.+)`)

	text := "Empresa: ../../etc\nNota: ../../passwd"

	relPath, unmatched := ResolveDestination(text, levels, filenameRegex)
	if unmatched != nil {
		t.Fatalf("não esperava falha: %+v", unmatched)
	}
	if relPath == "" {
		t.Fatal("relPath não deveria ser vazio")
	}
	cleaned := filepath.Clean(relPath)
	if filepath.IsAbs(cleaned) {
		t.Fatalf("relPath não pode ser absoluto: %q", relPath)
	}
	rel, err := filepath.Rel(".", cleaned)
	if err != nil {
		t.Fatalf("filepath.Rel: %v", err)
	}
	if rel == ".." || len(rel) >= 2 && rel[:2] == ".." {
		t.Fatalf("relPath escapa do diretório de destino: %q (cleaned=%q)", relPath, cleaned)
	}
}

func TestResolveDestinationSlashInCaptureIsSanitized(t *testing.T) {
	levels := []Level{
		{Label: "empresa", Regex: regexp.MustCompile(`Empresa: (.+)`)},
	}

	text := "Empresa: a/b/c"

	relPath, unmatched := ResolveDestination(text, levels, nil)
	if unmatched != nil {
		t.Fatalf("não esperava falha: %+v", unmatched)
	}
	// A captura "a/b/c" deve virar um único componente sanitizado, não 3
	// pastas aninhadas.
	if relPath != "a_b_c" {
		t.Fatalf("relPath = %q, esperava %q (barras sanitizadas dentro do componente)", relPath, "a_b_c")
	}
}

// TestResolveDestinationFilenameCaptureAlreadyHasExtensionNotDuplicated cobre
// o mesmo defeito de extensão duplicada corrigido em split.go, mas do lado
// de organize.go: se o grupo de captura de FilenameRegex casar um trecho que
// já termina em ".pdf" (situação incomum, mas possível dependendo da regex e
// do texto do documento), o nome final não pode virar "....pdf.pdf".
func TestResolveDestinationFilenameCaptureAlreadyHasExtensionNotDuplicated(t *testing.T) {
	filenameRegex := regexp.MustCompile(`Arquivo: (\S+)`)
	text := "Arquivo: relatorio.pdf"

	relPath, unmatched := ResolveDestination(text, nil, filenameRegex)
	if unmatched != nil {
		t.Fatalf("não esperava falha: %+v", unmatched)
	}
	if relPath != "relatorio.pdf" {
		t.Fatalf("relPath = %q, esperava %q (sem duplicar a extensão)", relPath, "relatorio.pdf")
	}
}

func TestOrganizeDryRunDoesNotTouchFiles(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	// Arquivos "PDF" falsos: extração de texto vai falhar para todos eles,
	// então devem cair em Unclassified — mas nenhum arquivo pode ser
	// criado/movido no diretório de saída, pois é dry-run.
	names := []string{"doc-a.pdf", "doc-b.pdf", "doc-c.pdf"}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(inputDir, n), []byte("não é um PDF de verdade"), 0o644); err != nil {
			t.Fatalf("criar arquivo de teste: %v", err)
		}
	}

	result, err := Organize(context.Background(), OrganizeOptions{
		InputDir:  inputDir,
		OutputDir: outputDir,
		Levels: []Level{
			{Label: "ano", Regex: regexp.MustCompile(`Ano: (\d{4})`)},
		},
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if result.Total != 3 {
		t.Fatalf("Total = %d, esperava 3", result.Total)
	}
	if len(result.Organized) != 0 {
		t.Fatalf("esperava 0 organizados (PDFs falsos falham na extração de texto), obteve %d", len(result.Organized))
	}
	if len(result.Unclassified) != 3 {
		t.Fatalf("esperava 3 não-classificados, obteve %d", len(result.Unclassified))
	}
	for _, e := range result.Unclassified {
		if e.Unmatched == nil || e.Unmatched.Level != "texto" {
			t.Errorf("entrada %+v deveria ter Unmatched.Level == \"texto\"", e)
		}
	}
	if !result.DryRun {
		t.Fatal("DryRun deveria ser true no resultado")
	}

	// Garantia central do dry-run: nada foi criado no diretório de saída.
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatalf("ler outputDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("outputDir deveria continuar vazio após dry-run, contém: %v", entries)
	}

	// E os arquivos de entrada continuam intactos.
	inputEntries, err := os.ReadDir(inputDir)
	if err != nil {
		t.Fatalf("ler inputDir: %v", err)
	}
	if len(inputEntries) != 3 {
		t.Fatalf("inputDir deveria continuar com 3 arquivos, tem %d", len(inputEntries))
	}
}

func TestOrganizeSummary(t *testing.T) {
	r := OrganizeResult{
		Organized:    make([]OrganizeEntry, 12),
		Unclassified: make([]OrganizeEntry, 3),
		Total:        15,
	}
	got := r.Summary()
	want := "12 de 15 arquivos organizados, 3 em sem-classificacao"
	if got != want {
		t.Fatalf("Summary() = %q, esperava %q", got, want)
	}

	r.DryRun = true
	got = r.Summary()
	if got != "[simulação] "+want {
		t.Fatalf("Summary() em dry-run = %q, esperava prefixo [simulação] ", got)
	}
}
