package pdfutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// --- ClassifyPages -----------------------------------------------------

func TestClassifyPages(t *testing.T) {
	tests := []struct {
		name       string
		pageTexts  []string
		imagesPage map[int]int
		want       []PageKind
	}{
		{
			name:       "so imagem sem texto: puro scan",
			pageTexts:  []string{""},
			imagesPage: map[int]int{1: 1},
			want:       []PageKind{PagePureScan},
		},
		{
			name:       "com texto embutido, sem imagem: ja tem texto",
			pageTexts:  []string{"NF: 00123"},
			imagesPage: map[int]int{},
			want:       []PageKind{PageHasText},
		},
		{
			name:       "duas imagens, sem texto: misto",
			pageTexts:  []string{""},
			imagesPage: map[int]int{1: 2},
			want:       []PageKind{PageMixed},
		},
		{
			name:       "imagem e texto juntos: misto",
			pageTexts:  []string{"algum texto"},
			imagesPage: map[int]int{1: 1},
			want:       []PageKind{PageMixed},
		},
		{
			name:       "nenhuma imagem e nenhum texto",
			pageTexts:  []string{""},
			imagesPage: map[int]int{},
			want:       []PageKind{PageNoImage},
		},
		{
			name:       "texto so com espacos em branco conta como sem texto",
			pageTexts:  []string{"   \n\t  "},
			imagesPage: map[int]int{1: 1},
			want:       []PageKind{PagePureScan},
		},
		{
			name:       "multiplas paginas mistas",
			pageTexts:  []string{"", "com texto", ""},
			imagesPage: map[int]int{1: 1, 2: 0, 3: 2},
			want:       []PageKind{PagePureScan, PageHasText, PageMixed},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyPages(tt.pageTexts, tt.imagesPage)
			if len(got) != len(tt.want) {
				t.Fatalf("ClassifyPages devolveu %d páginas, esperava %d", len(got), len(tt.want))
			}
			for i, p := range got {
				if p.Number != i+1 {
					t.Errorf("página %d: Number = %d, esperava %d", i, p.Number, i+1)
				}
				if p.Kind != tt.want[i] {
					t.Errorf("página %d: Kind = %v, esperava %v", i, p.Kind, tt.want[i])
				}
			}
		})
	}
}

// --- DecideEligibility ---------------------------------------------------

func TestDecideEligibility(t *testing.T) {
	tests := []struct {
		name          string
		pages         []PageInfo
		wantEligible  bool
		wantReasonHas string
	}{
		{
			name:         "todas puro scan: elegivel",
			pages:        []PageInfo{{Number: 1, Kind: PagePureScan}, {Number: 2, Kind: PagePureScan}},
			wantEligible: true,
		},
		{
			name:          "zero paginas: nao elegivel",
			pages:         nil,
			wantEligible:  false,
			wantReasonHas: "nenhuma página",
		},
		{
			name: "alguma pagina mista: nao elegivel",
			pages: []PageInfo{
				{Number: 1, Kind: PagePureScan},
				{Number: 2, Kind: PageMixed},
			},
			wantEligible:  false,
			wantReasonHas: "conteúdo além de uma única imagem",
		},
		{
			name: "todas com texto: nao elegivel (economia)",
			pages: []PageInfo{
				{Number: 1, Kind: PageHasText},
				{Number: 2, Kind: PageHasText},
			},
			wantEligible:  false,
			wantReasonHas: "já tem texto embutido em todas as páginas",
		},
		{
			name: "mistura de puro scan e com texto: nao elegivel",
			pages: []PageInfo{
				{Number: 1, Kind: PagePureScan},
				{Number: 2, Kind: PageHasText},
			},
			wantEligible:  false,
			wantReasonHas: "só parte do arquivo é digitalizada",
		},
		{
			name: "sem imagem junto de scans: nao elegivel",
			pages: []PageInfo{
				{Number: 1, Kind: PagePureScan},
				{Number: 2, Kind: PageNoImage},
			},
			wantEligible:  false,
			wantReasonHas: "não têm imagem nem texto",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecideEligibility("arquivo.pdf", tt.pages)
			if got.Eligible != tt.wantEligible {
				t.Fatalf("Eligible = %v, esperava %v (reason: %q)", got.Eligible, tt.wantEligible, got.Reason)
			}
			if !tt.wantEligible {
				if got.Reason == "" {
					t.Fatal("esperava motivo preenchido para arquivo não elegível")
				}
				if tt.wantReasonHas != "" && !strings.Contains(got.Reason, tt.wantReasonHas) {
					t.Errorf("Reason = %q, esperava conter %q", got.Reason, tt.wantReasonHas)
				}
			} else if got.Reason != "" {
				t.Errorf("Reason = %q, esperava vazio para arquivo elegível", got.Reason)
			}
		})
	}
}

// --- fakeSearchablePDFEngine ---------------------------------------------

// pagePDFPattern extrai o número de página do outBase gerado por
// ocrizeOneFile ("<tmp>/pagina-00007" -> 7), para que o motor falso possa
// embutir um marcador de página no PDF que produz — usado para provar que a
// ordem final do merge segue o NÚMERO da página, não o nome do arquivo de
// imagem extraída (ver TestOCRizePreservesPageOrderWithDoubleDigitPages).
var pagePDFPattern = regexp.MustCompile(`pagina-(\d+)$`)

// fakeSearchablePDFEngine implementa SearchablePDFEngine sem depender do
// tesseract: em vez de rodar OCR de verdade, grava em "<outBase>.pdf" um
// PDF mínimo (gerado por buildTestPDF, mesma fixture usada pelos testes de
// integração deste pacote) contendo um marcador textual do número de
// página — suficiente para que Merge (pdfcpu real) combine as páginas e
// para que os testes leiam de volta, via ExtractPageTexts, em que ordem
// elas ficaram no arquivo final.
type fakeSearchablePDFEngine struct {
	t         *testing.T
	available bool
	err       error
	calls     int
	callPaths []string
}

func (f *fakeSearchablePDFEngine) Available() bool { return f.available }

func (f *fakeSearchablePDFEngine) ImageToSearchablePDF(ctx context.Context, imagePath, outBase, lang string) error {
	f.calls++
	f.callPaths = append(f.callPaths, imagePath)
	if f.err != nil {
		return f.err
	}

	marker := "pagina-ocr"
	if m := pagePDFPattern.FindStringSubmatch(filepath.Base(outBase)); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			marker = fmt.Sprintf("PAGINA_%d", n)
		}
	}

	data := buildTestPDF(f.t, []string{marker})
	return os.WriteFile(outBase+".pdf", data, 0o644)
}

// --- OCRize: motor indisponível ------------------------------------------

func TestOCRizeEngineUnavailableFailsBeforeProcessing(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "scan.pdf")
	if err := os.WriteFile(srcPath, buildImagePDF(t), 0o644); err != nil {
		t.Fatalf("escrever PDF de teste: %v", err)
	}

	fake := &fakeSearchablePDFEngine{t: t, available: false}

	_, err := OCRize(context.Background(), OCRizeOptions{
		Inputs: []string{srcPath},
		Engine: fake,
	})
	if err == nil {
		t.Fatal("esperava erro com motor de OCR indisponível")
	}
	if fake.calls != 0 {
		t.Errorf("motor foi chamado %d vezes; esperava 0 (deveria falhar antes de processar qualquer arquivo)", fake.calls)
	}
}

func TestOCRizeNilEngineFails(t *testing.T) {
	_, err := OCRize(context.Background(), OCRizeOptions{Inputs: []string{"qualquer.pdf"}})
	if err == nil {
		t.Fatal("esperava erro com Engine nil")
	}
}

// --- OCRize: elegível vira Processed --------------------------------------

func TestOCRizeEligibleFileIsProcessed(t *testing.T) {
	ClearTextCache()
	t.Cleanup(ClearTextCache)

	dir := t.TempDir()
	srcPath := filepath.Join(dir, "scan.pdf")
	if err := os.WriteFile(srcPath, buildImagePDF(t), 0o644); err != nil {
		t.Fatalf("escrever PDF de teste: %v", err)
	}

	fake := &fakeSearchablePDFEngine{t: t, available: true}

	result, err := OCRize(context.Background(), OCRizeOptions{
		Inputs: []string{srcPath},
		Engine: fake,
	})
	if err != nil {
		t.Fatalf("OCRize: %v", err)
	}

	if len(result.Processed) != 1 {
		t.Fatalf("Processed = %d, esperava 1 (Skipped: %+v)", len(result.Processed), result.Skipped)
	}
	entry := result.Processed[0]
	if !entry.Done {
		t.Error("entry.Done deveria ser true")
	}
	if entry.Reason != "" {
		t.Errorf("entry.Reason = %q, esperava vazio", entry.Reason)
	}

	wantDest := filepath.Join(dir, "scan-ocr.pdf")
	if entry.Dest != wantDest {
		t.Errorf("Dest = %q, esperava %q", entry.Dest, wantDest)
	}
	if _, statErr := os.Stat(wantDest); statErr != nil {
		t.Fatalf("arquivo de saída não foi criado em %q: %v", wantDest, statErr)
	}

	// O original nunca é sobrescrito nem removido.
	if _, statErr := os.Stat(srcPath); statErr != nil {
		t.Fatalf("arquivo original deveria continuar existindo: %v", statErr)
	}
}

// --- OCRize: não elegível vira Skipped com motivo -------------------------

func TestOCRizeIneligibleFileIsSkippedWithReason(t *testing.T) {
	ClearTextCache()
	t.Cleanup(ClearTextCache)

	dir := t.TempDir()
	// buildTestPDF gera um PDF só com texto embutido, sem nenhuma imagem —
	// portanto PageHasText em todas as páginas: não elegível, "já tem
	// texto".
	srcPath := filepath.Join(dir, "com-texto.pdf")
	if err := os.WriteFile(srcPath, buildTestPDF(t, []string{"NF: 00123"}), 0o644); err != nil {
		t.Fatalf("escrever PDF de teste: %v", err)
	}

	fake := &fakeSearchablePDFEngine{t: t, available: true}

	result, err := OCRize(context.Background(), OCRizeOptions{
		Inputs: []string{srcPath},
		Engine: fake,
	})
	if err != nil {
		t.Fatalf("OCRize: %v", err)
	}

	if len(result.Skipped) != 1 {
		t.Fatalf("Skipped = %d, esperava 1 (Processed: %+v)", len(result.Skipped), result.Processed)
	}
	entry := result.Skipped[0]
	if entry.Done {
		t.Error("entry.Done deveria ser false")
	}
	if !strings.Contains(entry.Reason, "já tem texto embutido") {
		t.Errorf("Reason = %q, esperava conter \"já tem texto embutido\"", entry.Reason)
	}
	if fake.calls != 0 {
		t.Errorf("motor de OCR foi chamado %d vezes para um arquivo não elegível; esperava 0", fake.calls)
	}

	// Nenhum arquivo de saída deve ter sido criado.
	if _, statErr := os.Stat(filepath.Join(dir, "com-texto-ocr.pdf")); statErr == nil {
		t.Error("um arquivo de saída foi criado para um arquivo não elegível")
	}
}

// --- OCRize: DryRun não cria nada -----------------------------------------

func TestOCRizeDryRunCreatesNothing(t *testing.T) {
	ClearTextCache()
	t.Cleanup(ClearTextCache)

	dir := t.TempDir()
	srcPath := filepath.Join(dir, "scan.pdf")
	if err := os.WriteFile(srcPath, buildImagePDF(t), 0o644); err != nil {
		t.Fatalf("escrever PDF de teste: %v", err)
	}

	fake := &fakeSearchablePDFEngine{t: t, available: true}

	result, err := OCRize(context.Background(), OCRizeOptions{
		Inputs: []string{srcPath},
		Engine: fake,
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("OCRize: %v", err)
	}

	if len(result.Processed) != 1 || !result.Processed[0].Done {
		t.Fatalf("esperava 1 arquivo elegível marcado como Done em DryRun, obteve Processed=%+v Skipped=%+v", result.Processed, result.Skipped)
	}
	if !result.DryRun {
		t.Error("OCRizeResult.DryRun deveria ser true")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ler diretório: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("DryRun criou arquivo(s) além do original: %v", entries)
	}
	if fake.calls != 0 {
		t.Errorf("motor de OCR foi chamado %d vezes em DryRun; esperava 0", fake.calls)
	}
}

// --- OCRize: destino existente é pulado sem derrubar o lote ---------------

func TestOCRizeExistingDestinationSkippedWithoutOverwrite(t *testing.T) {
	ClearTextCache()
	t.Cleanup(ClearTextCache)

	dir := t.TempDir()
	srcPath := filepath.Join(dir, "scan.pdf")
	if err := os.WriteFile(srcPath, buildImagePDF(t), 0o644); err != nil {
		t.Fatalf("escrever PDF de teste: %v", err)
	}

	destPath := filepath.Join(dir, "scan-ocr.pdf")
	if err := os.WriteFile(destPath, []byte("conteudo anterior"), 0o644); err != nil {
		t.Fatalf("escrever destino pré-existente: %v", err)
	}

	fake := &fakeSearchablePDFEngine{t: t, available: true}

	result, err := OCRize(context.Background(), OCRizeOptions{
		Inputs: []string{srcPath},
		Engine: fake,
	})
	if err != nil {
		t.Fatalf("OCRize: %v", err)
	}

	if len(result.Skipped) != 1 {
		t.Fatalf("esperava 1 arquivo pulado por destino já existente, obteve Processed=%+v Skipped=%+v", result.Processed, result.Skipped)
	}
	if !strings.Contains(result.Skipped[0].Reason, "já existe") {
		t.Errorf("Reason = %q, esperava mencionar que o destino já existe", result.Skipped[0].Reason)
	}

	// O destino pré-existente não pode ter sido tocado.
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("ler destino: %v", err)
	}
	if string(data) != "conteudo anterior" {
		t.Error("o arquivo de destino pré-existente foi sobrescrito sem --overwrite")
	}
}

func TestOCRizeExistingDestinationSkipExisting(t *testing.T) {
	ClearTextCache()
	t.Cleanup(ClearTextCache)

	dir := t.TempDir()
	srcPath := filepath.Join(dir, "scan.pdf")
	if err := os.WriteFile(srcPath, buildImagePDF(t), 0o644); err != nil {
		t.Fatalf("escrever PDF de teste: %v", err)
	}
	destPath := filepath.Join(dir, "scan-ocr.pdf")
	if err := os.WriteFile(destPath, []byte("conteudo anterior"), 0o644); err != nil {
		t.Fatalf("escrever destino pré-existente: %v", err)
	}

	fake := &fakeSearchablePDFEngine{t: t, available: true}

	result, err := OCRize(context.Background(), OCRizeOptions{
		Inputs:       []string{srcPath},
		Engine:       fake,
		SkipExisting: true,
	})
	if err != nil {
		t.Fatalf("OCRize: %v", err)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("esperava 1 pulado, obteve Processed=%+v Skipped=%+v", result.Processed, result.Skipped)
	}
	if !strings.Contains(result.Skipped[0].Reason, "skip-existing") {
		t.Errorf("Reason = %q, esperava mencionar --skip-existing", result.Skipped[0].Reason)
	}
}

// --- OCRize: Progress chamado uma vez por arquivo -------------------------

func TestOCRizeProgressCalledOncePerFile(t *testing.T) {
	ClearTextCache()
	t.Cleanup(ClearTextCache)

	dir := t.TempDir()
	var paths []string
	for i := 1; i <= 3; i++ {
		p := filepath.Join(dir, fmt.Sprintf("scan%d.pdf", i))
		if err := os.WriteFile(p, buildImagePDF(t), 0o644); err != nil {
			t.Fatalf("escrever PDF de teste: %v", err)
		}
		paths = append(paths, p)
	}

	fake := &fakeSearchablePDFEngine{t: t, available: true}

	var progressCalls int
	var lastDone, lastTotal int
	result, err := OCRize(context.Background(), OCRizeOptions{
		Inputs: paths,
		Engine: fake,
		Progress: func(done, total int, path string) {
			progressCalls++
			lastDone, lastTotal = done, total
		},
	})
	if err != nil {
		t.Fatalf("OCRize: %v", err)
	}
	if len(result.Processed) != 3 {
		t.Fatalf("Processed = %d, esperava 3", len(result.Processed))
	}
	if progressCalls != 3 {
		t.Fatalf("Progress foi chamado %d vezes, esperava 3 (uma por arquivo)", progressCalls)
	}
	if lastDone != 3 || lastTotal != 3 {
		t.Errorf("última chamada de Progress = (%d, %d), esperava (3, 3)", lastDone, lastTotal)
	}
}

// --- OCRize: contexto cancelado interrompe o lote --------------------------

func TestOCRizeCanceledContextStopsBatch(t *testing.T) {
	ClearTextCache()
	t.Cleanup(ClearTextCache)

	dir := t.TempDir()
	var paths []string
	for i := 1; i <= 3; i++ {
		p := filepath.Join(dir, fmt.Sprintf("scan%d.pdf", i))
		if err := os.WriteFile(p, buildImagePDF(t), 0o644); err != nil {
			t.Fatalf("escrever PDF de teste: %v", err)
		}
		paths = append(paths, p)
	}

	fake := &fakeSearchablePDFEngine{t: t, available: true}

	ctx, cancel := context.WithCancel(context.Background())
	callCount := 0
	_, err := OCRize(ctx, OCRizeOptions{
		Inputs: paths,
		Engine: fake,
		Progress: func(done, total int, path string) {
			callCount++
			if callCount == 1 {
				cancel()
			}
		},
	})
	if err == nil {
		t.Fatal("esperava erro com contexto cancelado no meio do lote")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("erro = %v, esperava errors.Is(err, context.Canceled)", err)
	}
	if callCount != 1 {
		t.Errorf("Progress foi chamado %d vezes; esperava exatamente 1 antes da interrupção", callCount)
	}
}

// --- OCRize: ordem das páginas preservada ----------------------------------

// buildMultiPageImagePDF gera um PDF de n páginas, cada uma com exatamente
// uma imagem raster embutida (mesmo padrão de buildImagePDF, definida em
// organize_test.go, mesmo pacote) e sem texto — ou seja, n páginas
// PagePureScan. Usado para provar que a ordem final do arquivo unido segue
// o NÚMERO da página, não a ordem alfabética dos nomes de arquivo de
// imagem extraída pelo pdfcpu (que, para 10 páginas, ordena
// "..._1_...", "..._10_...", "..._2_..." — quebrando a ordem numérica).
func buildMultiPageImagePDF(t *testing.T, n int) []byte {
	t.Helper()
	b := newImgPDFBuilder()

	catalogID := b.reserveID()
	pagesID := b.reserveID()

	pageIDs := make([]int, n)
	for i := 0; i < n; i++ {
		imgData := []byte{0x00, 0x40, 0x80, 0xFF}
		imgID := b.addRawStreamObj("/Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8", imgData)
		contentID := b.addRawStreamObj("", []byte("q 50 0 0 50 10 10 cm /Im1 Do Q"))
		pageID := b.reserveID()
		pageIDs[i] = pageID
		b.placeObj(pageID, fmt.Sprintf(
			"<< /Type /Page /Parent %d 0 R /Resources << /XObject << /Im1 %d 0 R >> >> /MediaBox [0 0 612 792] /Contents %d 0 R >>",
			pagesID, imgID, contentID,
		))
	}

	var kids strings.Builder
	for _, id := range pageIDs {
		fmt.Fprintf(&kids, "%d 0 R ", id)
	}
	b.placeObj(pagesID, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids.String(), n))
	b.placeObj(catalogID, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pagesID))

	return b.finish(catalogID)
}

func TestOCRizePreservesPageOrderWithDoubleDigitPages(t *testing.T) {
	ClearTextCache()
	t.Cleanup(ClearTextCache)

	dir := t.TempDir()
	srcPath := filepath.Join(dir, "scan.pdf")
	if err := os.WriteFile(srcPath, buildMultiPageImagePDF(t, 10), 0o644); err != nil {
		t.Fatalf("escrever PDF de teste: %v", err)
	}

	fake := &fakeSearchablePDFEngine{t: t, available: true}

	result, err := OCRize(context.Background(), OCRizeOptions{
		Inputs: []string{srcPath},
		Engine: fake,
	})
	if err != nil {
		t.Fatalf("OCRize: %v", err)
	}
	if len(result.Processed) != 1 {
		t.Fatalf("Processed = %d, esperava 1 (Skipped: %+v)", len(result.Processed), result.Skipped)
	}

	dest := result.Processed[0].Dest
	texts, err := ExtractPageTexts(dest)
	if err != nil {
		t.Fatalf("ExtractPageTexts(%q): %v", dest, err)
	}
	if len(texts) != 10 {
		t.Fatalf("arquivo final tem %d páginas, esperava 10", len(texts))
	}
	for i, text := range texts {
		want := fmt.Sprintf("PAGINA_%d", i+1)
		if !strings.Contains(text, want) {
			t.Errorf("página %d do arquivo final = %q, esperava conter %q (ordem das páginas não preservada)", i+1, text, want)
		}
	}
}

// --- BuildOCRizeReport / WriteOCRizeReportCSV ------------------------------

func TestBuildOCRizeReportOrdersByFilename(t *testing.T) {
	result := OCRizeResult{
		Processed: []OCRizeEntry{
			{Source: "/x/zeta.pdf", Dest: "/x/zeta-ocr.pdf", Pages: 2, Done: true},
		},
		Skipped: []OCRizeEntry{
			{Source: "/x/alfa.pdf", Dest: "/x/alfa-ocr.pdf", Pages: 1, Reason: "já tem texto embutido em todas as páginas"},
		},
	}

	rows := BuildOCRizeReport(result)
	if len(rows) != 2 {
		t.Fatalf("BuildOCRizeReport devolveu %d linhas, esperava 2", len(rows))
	}
	if rows[0].Arquivo != "alfa.pdf" || rows[1].Arquivo != "zeta.pdf" {
		t.Fatalf("ordem das linhas = [%q, %q], esperava ordenado por nome de arquivo", rows[0].Arquivo, rows[1].Arquivo)
	}
	if rows[0].Processado {
		t.Error("alfa.pdf não deveria estar marcado como processado")
	}
	if !rows[1].Processado {
		t.Error("zeta.pdf deveria estar marcado como processado")
	}
}

func TestWriteOCRizeReportCSVHasBOMAndHeader(t *testing.T) {
	rows := []OCRizeReportRow{
		{Arquivo: "a.pdf", Origem: "/x/a.pdf", Destino: "/x/a-ocr.pdf", Processado: true, Paginas: 3},
	}

	var buf strings.Builder
	if err := WriteOCRizeReportCSV(&buf, rows); err != nil {
		t.Fatalf("WriteOCRizeReportCSV: %v", err)
	}

	out := buf.String()
	if !strings.HasPrefix(out, csvUTF8BOM) {
		t.Error("CSV não começa com o BOM UTF-8")
	}
	if !strings.Contains(out, "arquivo,origem,destino,processado,paginas,motivo") {
		t.Errorf("cabeçalho ausente ou incorreto: %q", out)
	}
	if !strings.Contains(out, "sim") {
		t.Error("coluna processado deveria conter \"sim\" para a linha processada")
	}
}
