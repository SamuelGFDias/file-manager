package pdfutil

import (
	"context"
	"fmt"
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
	_, err := ExtractPageTextsOpts(context.Background(), "/caminho/que/nao/existe.pdf", TextOptions{
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
	texts, err := ExtractPageTextsOpts(context.Background(), path, TextOptions{
		Mode:   OCRNever,
		Engine: engine,
	})
	if err != nil {
		t.Fatalf("ExtractPageTextsOpts: %v", err)
	}
	if len(texts) != 1 || !strings.Contains(texts[0], "primeira pagina") {
		t.Fatalf("texts = %#v, esperava conter o texto embutido", texts)
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
	texts, err := ExtractPageTextsOpts(context.Background(), path, TextOptions{
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

	first, err := ExtractPageTextsOpts(context.Background(), path, opts)
	if err != nil {
		t.Fatalf("primeira chamada: %v", err)
	}
	availableAfterFirst, _ := engine.counts()
	if availableAfterFirst == 0 {
		t.Fatal("esperava que Available fosse consultado na primeira chamada (nao cacheada)")
	}

	second, err := ExtractPageTextsOpts(context.Background(), path, opts)
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
	if _, err := ExtractPageTextsOpts(context.Background(), path, opts); err != nil {
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
			results[i], errs[i] = ExtractPageTextsOpts(context.Background(), path, opts)
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
