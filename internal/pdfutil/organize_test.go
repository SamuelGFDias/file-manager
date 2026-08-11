package pdfutil

import (
	"bytes"
	"context"
	"fmt"
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

// TestOrganizeZeroValueTextBehavesAsBefore confere que Organize com
// OrganizeOptions.Text no zero-value (Mode: "", Engine: nil) se comporta
// exatamente como o comportamento anterior à propagação de opções de OCR —
// mesmo cenário de TestIntegrationOrganizeCopiesNonDestructively (definido em
// integration_test.go, mesmo pacote de teste), agora com o campo Text
// explícito no zero-value, confirmando o mesmo resultado.
func TestOrganizeZeroValueTextBehavesAsBefore(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	srcPath := filepath.Join(inputDir, "documento.pdf")
	if err := os.WriteFile(srcPath, buildTestPDF(t, []string{"Empresa: Acme\nNF: 00789"}), 0o644); err != nil {
		t.Fatalf("escrever PDF de teste: %v", err)
	}

	result, err := Organize(context.Background(), OrganizeOptions{
		InputDir:  inputDir,
		OutputDir: outputDir,
		Levels: []Level{
			{Label: "empresa", Regex: regexp.MustCompile(`Empresa: (\w+)`)},
		},
		FilenameRegex: regexp.MustCompile(`NF:\s*(\d+)`),
		Copy:          true,
		Text:          TextOptions{}, // zero-value explícito
	})
	if err != nil {
		t.Fatalf("Organize: %v", err)
	}

	if len(result.Organized) != 1 {
		t.Fatalf("esperava 1 arquivo organizado, obteve %d (unclassified: %+v)", len(result.Organized), result.Unclassified)
	}
	wantDest := filepath.Join("Acme", "00789.pdf")
	if result.Organized[0].Dest != wantDest {
		t.Fatalf("Dest = %q, esperava %q", result.Organized[0].Dest, wantDest)
	}

	destAbs := filepath.Join(outputDir, wantDest)
	if _, err := os.Stat(destAbs); err != nil {
		t.Fatalf("arquivo de destino não foi criado em %q: %v", destAbs, err)
	}
	if _, err := os.Stat(srcPath); err != nil {
		t.Fatalf("arquivo original deveria continuar em %q (Copy, não Move): %v", srcPath, err)
	}
}

// TestOrganizeRecorderCalledOnRealMoveWithAllEntries prova que Recorder
// recebe TODAS as movimentações efetivas de uma execução real (--move),
// incluindo o arquivo que caiu em sem-classificacao — ele também foi
// movido, e precisa poder voltar.
func TestOrganizeRecorderCalledOnRealMoveWithAllEntries(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	classified := filepath.Join(inputDir, "classificado.pdf")
	if err := os.WriteFile(classified, buildTestPDF(t, []string{"Empresa: Acme"}), 0o644); err != nil {
		t.Fatalf("escrever pdf classificado: %v", err)
	}
	unclassified := filepath.Join(inputDir, "sem-info.pdf")
	if err := os.WriteFile(unclassified, []byte("nao e um pdf de verdade"), 0o644); err != nil {
		t.Fatalf("escrever pdf não classificável: %v", err)
	}

	var gotAction string
	var gotEntries []RecordedEntry

	result, err := Organize(context.Background(), OrganizeOptions{
		InputDir:  inputDir,
		OutputDir: outputDir,
		Levels: []Level{
			{Label: "empresa", Regex: regexp.MustCompile(`Empresa: (\w+)`)},
		},
		Copy: false, // --move
		Recorder: func(action string, entries []RecordedEntry) error {
			gotAction = action
			gotEntries = entries
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Organize: %v", err)
	}

	if gotAction != "move" {
		t.Fatalf("Recorder recebeu action = %q, esperava %q", gotAction, "move")
	}
	if len(gotEntries) != 2 {
		t.Fatalf("Recorder recebeu %d entradas, esperava 2 (1 classificada + 1 sem-classificacao); result=%+v", len(gotEntries), result)
	}

	for _, e := range gotEntries {
		if !filepath.IsAbs(e.Source) {
			t.Errorf("RecordedEntry.Source deveria ser absoluto, obteve %q", e.Source)
		}
		if !filepath.IsAbs(e.Dest) {
			t.Errorf("RecordedEntry.Dest deveria ser absoluto, obteve %q", e.Dest)
		}
		if e.Size <= 0 {
			t.Errorf("RecordedEntry.Size deveria ser > 0, obteve %d para %q", e.Size, e.Dest)
		}
		if _, statErr := os.Stat(e.Source); statErr == nil {
			t.Errorf("RecordedEntry.Source (%q) não deveria mais existir após --move", e.Source)
		}
		if _, statErr := os.Stat(e.Dest); statErr != nil {
			t.Errorf("RecordedEntry.Dest (%q) deveria existir: %v", e.Dest, statErr)
		}
	}
}

// TestOrganizeRecorderCalledOnRealCopy confirma que, com Copy: true, o
// Recorder recebe action = "copy".
func TestOrganizeRecorderCalledOnRealCopy(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(inputDir, "a.pdf"), buildTestPDF(t, []string{"Empresa: Acme"}), 0o644); err != nil {
		t.Fatalf("escrever pdf: %v", err)
	}

	var gotAction string
	_, err := Organize(context.Background(), OrganizeOptions{
		InputDir:  inputDir,
		OutputDir: outputDir,
		Levels: []Level{
			{Label: "empresa", Regex: regexp.MustCompile(`Empresa: (\w+)`)},
		},
		Copy: true,
		Recorder: func(action string, entries []RecordedEntry) error {
			gotAction = action
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Organize: %v", err)
	}
	if gotAction != "copy" {
		t.Fatalf("Recorder recebeu action = %q, esperava %q", gotAction, "copy")
	}
}

// TestOrganizeDryRunNeverCallsRecorder é a garantia central pedida: uma
// simulação não altera nada, então não pode gerar histórico.
func TestOrganizeDryRunNeverCallsRecorder(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(inputDir, "a.pdf"), buildTestPDF(t, []string{"Empresa: Acme"}), 0o644); err != nil {
		t.Fatalf("escrever pdf: %v", err)
	}

	called := false
	result, err := Organize(context.Background(), OrganizeOptions{
		InputDir:  inputDir,
		OutputDir: outputDir,
		Levels: []Level{
			{Label: "empresa", Regex: regexp.MustCompile(`Empresa: (\w+)`)},
		},
		DryRun: true,
		Recorder: func(action string, entries []RecordedEntry) error {
			called = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Organize: %v", err)
	}
	if called {
		t.Fatal("Recorder não deveria ser chamado em DryRun")
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("esperava 0 avisos em DryRun, obteve %+v", result.Warnings)
	}
}

// TestOrganizeRecorderNotCalledWhenNothingOrganized confirma que uma
// execução real que não organiza nada (pasta de entrada vazia) também não
// gera histórico, mesmo sem ser DryRun.
func TestOrganizeRecorderNotCalledWhenNothingOrganized(t *testing.T) {
	inputDir := t.TempDir() // vazio, de propósito
	outputDir := t.TempDir()

	called := false
	result, err := Organize(context.Background(), OrganizeOptions{
		InputDir:  inputDir,
		OutputDir: outputDir,
		Recorder: func(action string, entries []RecordedEntry) error {
			called = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Organize: %v", err)
	}
	if called {
		t.Fatal("Recorder não deveria ser chamado quando nada foi organizado")
	}
	if result.Total != 0 {
		t.Fatalf("Total = %d, esperava 0", result.Total)
	}
}

// TestOrganizeRecorderErrorBecomesWarningNotFailure prova a regra mais
// importante da injeção: uma falha ao gravar o histórico não pode falhar a
// operação de organizar, que já aconteceu de verdade.
func TestOrganizeRecorderErrorBecomesWarningNotFailure(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(inputDir, "a.pdf"), buildTestPDF(t, []string{"Empresa: Acme"}), 0o644); err != nil {
		t.Fatalf("escrever pdf: %v", err)
	}

	recorderErr := fmt.Errorf("disco cheio (erro simulado)")
	result, err := Organize(context.Background(), OrganizeOptions{
		InputDir:  inputDir,
		OutputDir: outputDir,
		Levels: []Level{
			{Label: "empresa", Regex: regexp.MustCompile(`Empresa: (\w+)`)},
		},
		Copy: true,
		Recorder: func(action string, entries []RecordedEntry) error {
			return recorderErr
		},
	})
	if err != nil {
		t.Fatalf("Organize não deveria falhar por causa de um erro do Recorder, obteve: %v", err)
	}
	if len(result.Organized) != 1 {
		t.Fatalf("a organização em si deveria ter acontecido normalmente, obteve %d organizados", len(result.Organized))
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("esperava exatamente 1 aviso sobre a falha do Recorder, obteve %+v", result.Warnings)
	}

	// O arquivo de destino precisa existir de verdade — a falha foi só no
	// registro do histórico, não na operação de organizar.
	destAbs := filepath.Join(outputDir, "Acme", "a.pdf")
	if _, statErr := os.Stat(destAbs); statErr != nil {
		t.Fatalf("arquivo de destino deveria existir apesar do erro do Recorder: %v", statErr)
	}
}

// --- PDF de teste com imagem embutida ---------------------------------------
//
// O motor de OCR falso usado abaixo (fakeOCREngine) já existe neste pacote de
// teste, definido em textextract_test.go — reaproveitado aqui em vez de
// redeclarado, para não colidir com aquele arquivo (que não deve ser
// alterado). Ele conta chamadas a Available/ImageToText via counts(), o que
// serve tanto para o pedido original ("provar que o fake foi de fato
// chamado") quanto para as asserções abaixo.

// imgPDFBuilder monta, byte a byte, um PDF válido mínimo com uma única página
// contendo uma imagem raster embutida (sem filtro, escala de cinza 8bpc) —
// necessária para exercitar o caminho de api.ExtractImagesFile usado pelo
// fallback de OCR (applyOCRFallback, em textextract.go). É deliberadamente
// separado do pdfBuilder de integration_test.go (que não gera imagens) para
// não alterar aquele arquivo.
type imgPDFBuilder struct {
	buf     bytes.Buffer
	offsets []int
}

func newImgPDFBuilder() *imgPDFBuilder {
	b := &imgPDFBuilder{}
	b.buf.WriteString("%PDF-1.4\n")
	return b
}

func (b *imgPDFBuilder) reserveID() int {
	id := len(b.offsets) + 1
	b.offsets = append(b.offsets, -1)
	return id
}

func (b *imgPDFBuilder) placeObj(id int, body string) {
	b.offsets[id-1] = b.buf.Len()
	fmt.Fprintf(&b.buf, "%d 0 obj\n%s\nendobj\n", id, body)
}

func (b *imgPDFBuilder) addObj(body string) int {
	id := b.reserveID()
	b.placeObj(id, body)
	return id
}

// addRawStreamObj escreve um objeto stream sem filtro (dictExtra some às
// entradas fixas do dicionário; content é o conteúdo cru do stream).
func (b *imgPDFBuilder) addRawStreamObj(dictExtra string, content []byte) int {
	id := len(b.offsets) + 1
	b.offsets = append(b.offsets, b.buf.Len())
	fmt.Fprintf(&b.buf, "%d 0 obj\n<< %s /Length %d >>\nstream\n", id, dictExtra, len(content))
	b.buf.Write(content)
	b.buf.WriteString("\nendstream\nendobj\n")
	return id
}

func (b *imgPDFBuilder) finish(rootID int) []byte {
	xrefOffset := b.buf.Len()
	b.buf.WriteString("xref\n")
	fmt.Fprintf(&b.buf, "0 %d\n", len(b.offsets)+1)
	b.buf.WriteString("0000000000 65535 f \n")
	for _, off := range b.offsets {
		fmt.Fprintf(&b.buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b.buf, "trailer\n<< /Size %d /Root %d 0 R >>\nstartxref\n%d\n%%%%EOF",
		len(b.offsets)+1, rootID, xrefOffset)
	return b.buf.Bytes()
}

// buildImagePDF gera um PDF de uma página com uma imagem raster 2x2 em escala
// de cinza (sem camada de texto embutida útil) embutida via /XObject, para
// que api.ExtractImagesFile (chamado por applyOCRFallback quando Mode !=
// OCRNever) encontre algo para extrair.
func buildImagePDF(t *testing.T) []byte {
	t.Helper()
	b := newImgPDFBuilder()

	catalogID := b.reserveID()
	pagesID := b.reserveID()

	imgData := []byte{0x00, 0x40, 0x80, 0xFF}
	imgID := b.addRawStreamObj("/Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8", imgData)

	contentID := b.addRawStreamObj("", []byte("q 50 0 0 50 10 10 cm /Im1 Do Q"))

	pageID := b.reserveID()
	b.placeObj(pageID, fmt.Sprintf(
		"<< /Type /Page /Parent %d 0 R /Resources << /XObject << /Im1 %d 0 R >> >> /MediaBox [0 0 612 792] /Contents %d 0 R >>",
		pagesID, imgID, contentID,
	))

	b.placeObj(pagesID, fmt.Sprintf("<< /Type /Pages /Kids [%d 0 R] /Count 1 >>", pageID))
	b.placeObj(catalogID, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pagesID))

	return b.finish(catalogID)
}

// TestOrganizeOCRAlwaysPropagatesFakeEngineTextToClassification é o teste de
// ponta a ponta pedido: prova que OrganizeOptions.Text chega até
// ExtractTextOpts (dentro de Organize) e que o texto devolvido pelo motor de
// OCR configurado é, de fato, o texto usado por ResolveDestination para
// classificar o arquivo. Sem a propagação feita em organize.go, opts.Text
// nunca chegaria ao motor falso e este teste falharia (Organize cairia no
// caminho "sem OCR" e o PDF, sem texto embutido, iria para
// sem-classificacao).
func TestOrganizeOCRAlwaysPropagatesFakeEngineTextToClassification(t *testing.T) {
	ClearTextCache()
	t.Cleanup(ClearTextCache)

	inputDir := t.TempDir()
	outputDir := t.TempDir()

	srcPath := filepath.Join(inputDir, "digitalizado.pdf")
	if err := os.WriteFile(srcPath, buildImagePDF(t), 0o644); err != nil {
		t.Fatalf("escrever PDF de teste com imagem: %v", err)
	}

	fake := &fakeOCREngine{available: true, text: "FORNECEDOR: ACME\nNF: 00123"}

	result, err := Organize(context.Background(), OrganizeOptions{
		InputDir:  inputDir,
		OutputDir: outputDir,
		Levels: []Level{
			{Label: "fornecedor", Regex: regexp.MustCompile(`FORNECEDOR: (\w+)`)},
		},
		FilenameRegex: regexp.MustCompile(`NF:\s*(\d+)`),
		Copy:          true,
		Text:          TextOptions{Mode: OCRAlways, Engine: fake},
	})
	if err != nil {
		t.Fatalf("Organize: %v", err)
	}

	if _, imageToTextCalls := fake.counts(); imageToTextCalls == 0 {
		t.Fatal("o motor de OCR falso nunca foi chamado; a extração de imagem pode ter falhado antes de acionar o OCR")
	}

	if len(result.Organized) != 1 {
		t.Fatalf("esperava 1 arquivo organizado via texto do OCR falso, obteve %d organizados e %d não-classificados (%+v)",
			len(result.Organized), len(result.Unclassified), result.Unclassified)
	}
	wantDest := filepath.Join("ACME", "00123.pdf")
	if result.Organized[0].Dest != wantDest {
		t.Fatalf("Dest = %q, esperava %q (classificado a partir do texto devolvido pelo motor falso)", result.Organized[0].Dest, wantDest)
	}

	destAbs := filepath.Join(outputDir, wantDest)
	if _, err := os.Stat(destAbs); err != nil {
		t.Fatalf("arquivo de destino não foi criado em %q: %v", destAbs, err)
	}
}
