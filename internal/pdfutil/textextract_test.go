package pdfutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// --- motor de OCR falso ------------------------------------------------
//
// fakeOCREngine implementa OCREngine sem depender de tesseract nem de
// processo externo. Conta chamadas a Available e ImageToText de forma
// segura para concorrência (a suíte roda com -race), o que permite às
// asserções checar se o cache evitou reprocessamento.

type fakeOCREngine struct {
	mu               sync.Mutex
	available        bool
	availableCalls   int
	imageToTextCalls int
	text             string
	err              error
}

func (f *fakeOCREngine) Available() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.availableCalls++
	return f.available
}

func (f *fakeOCREngine) ImageToText(ctx context.Context, imagePath, lang string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.imageToTextCalls++
	if f.err != nil {
		return "", f.err
	}
	return f.text, nil
}

func (f *fakeOCREngine) counts() (availableCalls, imageToTextCalls int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.availableCalls, f.imageToTextCalls
}

// --- ParseExtractedImageName --------------------------------------------

func TestParseExtractedImageName(t *testing.T) {
	tests := []struct {
		name      string
		filename  string
		wantPage  int
		wantIndex int
		wantOK    bool
	}{
		{
			name:      "padrao simples",
			filename:  "padrao1_1_Im0.png",
			wantPage:  1,
			wantIndex: 0,
			wantOK:    true,
		},
		{
			name:      "nome base com underscore e digitos - caso critico",
			filename:  "nota_2024_01_3_Im2.jpg",
			wantPage:  3,
			wantIndex: 2,
			wantOK:    true,
		},
		{
			name:     "nome sem o padrao de pagina/imagem",
			filename: "arquivo.pdf",
			wantOK:   false,
		},
		{
			name:     "grupos nao numericos",
			filename: "x_a_Imb.png",
			wantOK:   false,
		},
		{
			name:      "extensao maiuscula",
			filename:  "a_1_Im0.PNG",
			wantPage:  1,
			wantIndex: 0,
			wantOK:    true,
		},
		{
			name:      "prefixo X em vez de Im - caso real encontrado em auditoria",
			filename:  "scan2p_2_X0.jpg",
			wantPage:  2,
			wantIndex: 0,
			wantOK:    true,
		},
		{
			name:      "outro prefixo qualquer",
			filename:  "doc_5_Fm2.tif",
			wantPage:  5,
			wantIndex: 2,
			wantOK:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, index, ok := ParseExtractedImageName(tt.filename)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, esperava %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if page != tt.wantPage {
				t.Errorf("page = %d, esperava %d", page, tt.wantPage)
			}
			if index != tt.wantIndex {
				t.Errorf("index = %d, esperava %d", index, tt.wantIndex)
			}
		})
	}
}

// --- ParseOCRMode ---------------------------------------------------------

func TestParseOCRMode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    OCRMode
		wantErr bool
	}{
		{name: "auto", input: "auto", want: OCRAuto},
		{name: "always", input: "always", want: OCRAlways},
		{name: "never", input: "never", want: OCRNever},
		{name: "vazio vira auto", input: "", want: OCRAuto},
		{name: "sinonimo sempre", input: "sempre", want: OCRAlways},
		{name: "sinonimo nunca", input: "nunca", want: OCRNever},
		{name: "invalido", input: "qualquer-coisa", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseOCRMode(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseOCRMode(%q): esperava erro, obteve nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseOCRMode(%q): erro inesperado: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseOCRMode(%q) = %q, esperava %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- ExtractPageTextsOpts --------------------------------------------------

func TestExtractPageTextsOptsNilEngineNonexistentFileReturnsError(t *testing.T) {
	ClearTextCache()
	_, _, err := ExtractPageTextsOpts(context.Background(), "/caminho/que/nao/existe.pdf", TextOptions{
		Mode:   OCRAuto,
		Engine: nil,
	})
	if err == nil {
		t.Fatal("esperava erro para arquivo inexistente, obteve nil")
	}
}

func TestExtractPageTextsOptsNeverDoesNotCallEngine(t *testing.T) {
	ClearTextCache()
	dir := t.TempDir()
	path := writeTestPDF(t, dir, "entrada.pdf", []string{"primeira pagina"})

	engine := &fakeOCREngine{available: true, text: "nao deveria aparecer"}
	texts, warnings, err := ExtractPageTextsOpts(context.Background(), path, TextOptions{
		Mode:   OCRNever,
		Engine: engine,
	})
	if err != nil {
		t.Fatalf("ExtractPageTextsOpts: %v", err)
	}
	if len(texts) != 1 || !strings.Contains(texts[0], "primeira pagina") {
		t.Fatalf("texts = %#v, esperava conter o texto embutido", texts)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %#v, esperava nenhum em Mode: OCRNever", warnings)
	}

	availableCalls, imageToTextCalls := engine.counts()
	if availableCalls != 0 {
		t.Errorf("Available foi chamado %d vez(es) em Mode: OCRNever, esperava 0", availableCalls)
	}
	if imageToTextCalls != 0 {
		t.Errorf("ImageToText foi chamado %d vez(es) em Mode: OCRNever, esperava 0", imageToTextCalls)
	}
}

func TestExtractPageTextsOptsEngineUnavailableIsSilentlySkipped(t *testing.T) {
	ClearTextCache()
	dir := t.TempDir()
	path := writeTestPDF(t, dir, "entrada.pdf", []string{"primeira pagina"})

	engine := &fakeOCREngine{available: false, text: "nao deveria aparecer"}
	texts, _, err := ExtractPageTextsOpts(context.Background(), path, TextOptions{
		Mode:   OCRAuto,
		Engine: engine,
	})
	if err != nil {
		t.Fatalf("ExtractPageTextsOpts: esperava degradacao silenciosa, obteve erro: %v", err)
	}
	if len(texts) != 1 {
		t.Fatalf("texts = %#v, esperava 1 pagina", texts)
	}

	_, imageToTextCalls := engine.counts()
	if imageToTextCalls != 0 {
		t.Errorf("ImageToText foi chamado %d vez(es) com motor indisponivel, esperava 0", imageToTextCalls)
	}
}

func TestExtractPageTextsOptsCache(t *testing.T) {
	ClearTextCache()
	dir := t.TempDir()
	path := writeTestPDF(t, dir, "entrada.pdf", []string{"primeira pagina", "segunda pagina"})

	engine := &fakeOCREngine{available: true, text: "ocr"}
	opts := TextOptions{Mode: OCRAuto, Engine: engine}

	first, _, err := ExtractPageTextsOpts(context.Background(), path, opts)
	if err != nil {
		t.Fatalf("primeira chamada: %v", err)
	}
	availableAfterFirst, _ := engine.counts()
	if availableAfterFirst == 0 {
		t.Fatal("esperava que Available fosse consultado na primeira chamada (nao cacheada)")
	}

	second, _, err := ExtractPageTextsOpts(context.Background(), path, opts)
	if err != nil {
		t.Fatalf("segunda chamada: %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("resultados de tamanhos diferentes: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("pagina %d: %q != %q", i, first[i], second[i])
		}
	}

	availableAfterSecond, _ := engine.counts()
	if availableAfterSecond != availableAfterFirst {
		t.Errorf("segunda chamada reprocessou (Available chamado de novo): %d -> %d", availableAfterFirst, availableAfterSecond)
	}

	// ClearTextCache deve forcar reprocessamento.
	ClearTextCache()
	if _, _, err := ExtractPageTextsOpts(context.Background(), path, opts); err != nil {
		t.Fatalf("terceira chamada (pos ClearTextCache): %v", err)
	}
	availableAfterClear, _ := engine.counts()
	if availableAfterClear <= availableAfterSecond {
		t.Errorf("ClearTextCache nao forcou reprocessamento: Available continuou em %d", availableAfterClear)
	}
}

func TestExtractPageTextsOptsConcurrencySafe(t *testing.T) {
	ClearTextCache()
	dir := t.TempDir()
	path := writeTestPDF(t, dir, "entrada.pdf", []string{"primeira pagina", "segunda pagina"})

	engine := &fakeOCREngine{available: true, text: "ocr"}
	opts := TextOptions{Mode: OCRAuto, Engine: engine}

	const goroutines = 20
	results := make([][]string, goroutines)
	errs := make([]error, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			results[i], _, errs[i] = ExtractPageTextsOpts(context.Background(), path, opts)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: erro inesperado: %v", i, err)
		}
	}

	want := results[0]
	for i, got := range results {
		if len(got) != len(want) {
			t.Fatalf("goroutine %d: tamanho %d != %d", i, len(got), len(want))
		}
		for p := range want {
			if got[p] != want[p] {
				t.Fatalf("goroutine %d, pagina %d: %q != %q", i, p, got[p], want[p])
			}
		}
	}
}

// --- ExtractPageTexts / ExtractText (compatibilidade) ---------------------

// TestExtractPageTextsCompatibilitySignatureUnchanged confere que a função
// antiga continua se comportando como antes da introdução do OCR: nenhuma
// alteração de assinatura, sem fallback de OCR (equivalente a Mode: OCRNever).
func TestExtractPageTextsCompatibilitySignatureUnchanged(t *testing.T) {
	ClearTextCache()
	dir := t.TempDir()
	path := writeTestPDF(t, dir, "entrada.pdf", []string{"primeira pagina", "segunda pagina"})

	texts, err := ExtractPageTexts(path)
	if err != nil {
		t.Fatalf("ExtractPageTexts: %v", err)
	}
	if len(texts) != 2 {
		t.Fatalf("ExtractPageTexts devolveu %d paginas, esperava 2", len(texts))
	}
	if !strings.Contains(texts[0], "primeira pagina") {
		t.Errorf("texto da pagina 1 = %q, esperava conter %q", texts[0], "primeira pagina")
	}
	if !strings.Contains(texts[1], "segunda pagina") {
		t.Errorf("texto da pagina 2 = %q, esperava conter %q", texts[1], "segunda pagina")
	}

	full, err := ExtractText(path)
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	want := fmt.Sprintf("%s\n%s", texts[0], texts[1])
	if full != want {
		t.Errorf("ExtractText = %q, esperava %q", full, want)
	}
}

// --- imagens extraídas com prefixo diferente de "Im" (bug de auditoria) ---
//
// Os helpers abaixo reaproveitam imgPDFBuilder/newImgPDFBuilder, definidos
// em organize_test.go (mesmo pacote), parametrizando o nome do recurso
// XObject da imagem. Isso permite reproduzir com uma extração REAL do
// pdfcpu (não simulada) o caso relatado em auditoria: o pdfcpu deriva o
// nome do arquivo extraído do nome do recurso XObject da página (ver
// WriteImageToDisk em pdfcpu/pkg/api/extract.go, campo img.Name), que nem
// sempre é "Im<n>".

// buildImagePDFWithResourceName monta um PDF de uma página com uma única
// imagem raster embutida cujo recurso XObject se chama resourceName.
func buildImagePDFWithResourceName(t *testing.T, resourceName string) []byte {
	t.Helper()
	b := newImgPDFBuilder()

	catalogID := b.reserveID()
	pagesID := b.reserveID()

	imgData := []byte{0x00, 0x40, 0x80, 0xFF}
	imgID := b.addRawStreamObj("/Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8", imgData)

	contentID := b.addRawStreamObj("", []byte(fmt.Sprintf("q 50 0 0 50 10 10 cm /%s Do Q", resourceName)))

	pageID := b.reserveID()
	b.placeObj(pageID, fmt.Sprintf(
		"<< /Type /Page /Parent %d 0 R /Resources << /XObject << /%s %d 0 R >> >> /MediaBox [0 0 612 792] /Contents %d 0 R >>",
		pagesID, resourceName, imgID, contentID,
	))

	b.placeObj(pagesID, fmt.Sprintf("<< /Type /Pages /Kids [%d 0 R] /Count 1 >>", pageID))
	b.placeObj(catalogID, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pagesID))

	return b.finish(catalogID)
}

// buildImagePDFTwoResourceNames monta um PDF de uma página com DUAS imagens
// raster embutidas (dois objetos distintos), nomeadas name1 e name2 — usado
// para provar que uma imagem com nome não reconhecido não impede que a(s)
// outra(s), reconhecida(s), continuem sendo processadas normalmente.
func buildImagePDFTwoResourceNames(t *testing.T, name1, name2 string) []byte {
	t.Helper()
	b := newImgPDFBuilder()

	catalogID := b.reserveID()
	pagesID := b.reserveID()

	// Dados de imagem DIFERENTES entre as duas: o pass de otimização do
	// pdfcpu deduplica objetos de imagem byte-a-byte idênticos, o que
	// colapsaria os dois XObjects num só e esconderia o cenário que este
	// teste quer provar (duas imagens extraídas, uma reconhecida e outra
	// não).
	img1ID := b.addRawStreamObj("/Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8", []byte{0x00, 0x40, 0x80, 0xFF})
	img2ID := b.addRawStreamObj("/Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8", []byte{0x11, 0x22, 0x33, 0x44})

	contentID := b.addRawStreamObj("", []byte(fmt.Sprintf(
		"q 50 0 0 50 10 10 cm /%s Do Q q 50 0 0 50 100 10 cm /%s Do Q", name1, name2,
	)))

	pageID := b.reserveID()
	b.placeObj(pageID, fmt.Sprintf(
		"<< /Type /Page /Parent %d 0 R /Resources << /XObject << /%s %d 0 R /%s %d 0 R >> >> /MediaBox [0 0 612 792] /Contents %d 0 R >>",
		pagesID, name1, img1ID, name2, img2ID, contentID,
	))

	b.placeObj(pagesID, fmt.Sprintf("<< /Type /Pages /Kids [%d 0 R] /Count 1 >>", pageID))
	b.placeObj(catalogID, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pagesID))

	return b.finish(catalogID)
}

// TestExtractPageTextsOptsAcceptsNonImResourceName prova, com uma extração
// REAL do pdfcpu (não simulada), que uma imagem cujo recurso XObject não se
// chama "Im<n>" — o caso relatado em auditoria, onde a segunda página de um
// PDF real de duas páginas saiu nomeada "X0" e era descartada em silêncio —
// é reconhecida e enviada ao motor de OCR normalmente.
func TestExtractPageTextsOptsAcceptsNonImResourceName(t *testing.T) {
	ClearTextCache()
	dir := t.TempDir()
	path := filepath.Join(dir, "digitalizado.pdf")
	if err := os.WriteFile(path, buildImagePDFWithResourceName(t, "X0"), 0o644); err != nil {
		t.Fatalf("escrever PDF de teste: %v", err)
	}

	engine := &fakeOCREngine{available: true, text: "texto via ocr com prefixo X"}
	texts, warnings, err := ExtractPageTextsOpts(context.Background(), path, TextOptions{
		Mode:   OCRAlways,
		Engine: engine,
	})
	if err != nil {
		t.Fatalf("ExtractPageTextsOpts: %v", err)
	}
	if len(texts) != 1 || !strings.Contains(texts[0], "texto via ocr com prefixo X") {
		t.Fatalf("texts = %#v, esperava conter o texto devolvido pelo OCR", texts)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %#v, esperava nenhum (o prefixo X0 devia ser reconhecido)", warnings)
	}
	_, imageToTextCalls := engine.counts()
	if imageToTextCalls != 1 {
		t.Errorf("ImageToText foi chamado %d vez(es), esperava 1", imageToTextCalls)
	}
}

// TestExtractPageTextsOptsWarnsOnUnmatchedImageName confere que uma imagem
// extraída cujo nome não casa com o padrão gera um aviso citando o nome do
// arquivo e dizendo que ela foi ignorada — em vez de desaparecer em
// silêncio, que era o defeito original. Como esta é a única imagem da
// página, também confere o aviso agregado de "nenhuma imagem pôde ser
// associada".
func TestExtractPageTextsOptsWarnsOnUnmatchedImageName(t *testing.T) {
	ClearTextCache()
	dir := t.TempDir()
	path := filepath.Join(dir, "digitalizado.pdf")
	// "Foto" não termina em dígito: não casa com "<letras><índice numérico>".
	if err := os.WriteFile(path, buildImagePDFWithResourceName(t, "Foto"), 0o644); err != nil {
		t.Fatalf("escrever PDF de teste: %v", err)
	}

	engine := &fakeOCREngine{available: true, text: "nao deveria aparecer"}
	texts, warnings, err := ExtractPageTextsOpts(context.Background(), path, TextOptions{
		Mode:   OCRAlways,
		Engine: engine,
	})
	if err != nil {
		t.Fatalf("ExtractPageTextsOpts: %v", err)
	}
	if len(texts) != 1 || texts[0] != "" {
		t.Fatalf("texts = %#v, esperava página 1 vazia (nenhuma imagem foi reconhecida)", texts)
	}

	if len(warnings) != 2 {
		t.Fatalf("warnings = %#v, esperava 2 avisos (individual + agregado)", warnings)
	}
	var foundIndividual, foundAggregate bool
	for _, w := range warnings {
		if strings.Contains(w, "digitalizado_1_Foto") && strings.Contains(w, "ignorada") {
			foundIndividual = true
		}
		if strings.Contains(w, "nenhuma") && strings.Contains(w, "associada") {
			foundAggregate = true
		}
	}
	if !foundIndividual {
		t.Errorf("warnings = %#v, esperava aviso citando o nome do arquivo ignorado", warnings)
	}
	if !foundAggregate {
		t.Errorf("warnings = %#v, esperava aviso agregado de nenhuma imagem associada", warnings)
	}

	_, imageToTextCalls := engine.counts()
	if imageToTextCalls != 0 {
		t.Errorf("ImageToText foi chamado %d vez(es), esperava 0 (imagem nao reconhecida)", imageToTextCalls)
	}
}

// TestExtractPageTextsOptsUnmatchedImageDoesNotBlockMatchedOnes confere que,
// quando uma página tem duas imagens extraídas e só uma tem nome
// reconhecível, a reconhecível continua sendo processada normalmente: o
// aviso da outra não contamina o resultado nem impede o OCR de rodar, e o
// aviso agregado de "nenhuma imagem associada" NÃO aparece (pelo menos uma
// foi).
func TestExtractPageTextsOptsUnmatchedImageDoesNotBlockMatchedOnes(t *testing.T) {
	ClearTextCache()
	dir := t.TempDir()
	path := filepath.Join(dir, "digitalizado.pdf")
	if err := os.WriteFile(path, buildImagePDFTwoResourceNames(t, "Im0", "Foto"), 0o644); err != nil {
		t.Fatalf("escrever PDF de teste: %v", err)
	}

	engine := &fakeOCREngine{available: true, text: "texto reconhecido"}
	texts, warnings, err := ExtractPageTextsOpts(context.Background(), path, TextOptions{
		Mode:   OCRAlways,
		Engine: engine,
	})
	if err != nil {
		t.Fatalf("ExtractPageTextsOpts: %v", err)
	}
	if len(texts) != 1 || !strings.Contains(texts[0], "texto reconhecido") {
		t.Fatalf("texts = %#v, esperava conter o texto OCR da imagem reconhecida", texts)
	}

	if len(warnings) != 1 {
		t.Fatalf("warnings = %#v, esperava exatamente 1 (só o arquivo não reconhecido, sem o agregado)", warnings)
	}
	if !strings.Contains(warnings[0], "Foto") {
		t.Errorf("warnings[0] = %q, esperava citar o nome do arquivo não reconhecido", warnings[0])
	}

	_, imageToTextCalls := engine.counts()
	if imageToTextCalls != 1 {
		t.Errorf("ImageToText foi chamado %d vez(es), esperava 1 (só a imagem reconhecida)", imageToTextCalls)
	}
}
