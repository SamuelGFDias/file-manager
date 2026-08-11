package splitpdf

import (
	"strings"
	"testing"

	"github.com/SamuelGFDias/file-manager/internal/pdfutil"
	"github.com/spf13/cobra"
)

func TestMetaID(t *testing.T) {
	got := New().Meta().ID
	want := "split-pdf"
	if got != want {
		t.Fatalf("Meta().ID = %q, want %q", got, want)
	}
}

func TestCommandUse(t *testing.T) {
	got := New().Command().Use
	want := "split-pdf"
	if got != want {
		t.Fatalf("Command().Use = %q, want %q", got, want)
	}
}

func TestDocFlagsMatchesParams(t *testing.T) {
	tl := New()

	doc := tl.Doc()
	params := tl.params()
	cmd := tl.Command()

	if len(doc.Flags) != len(params) {
		t.Fatalf("Doc().Flags tem %d entradas, esperava %d (mesmo tamanho de params())", len(doc.Flags), len(params))
	}

	for _, fd := range doc.Flags {
		if cmd.Flags().Lookup(fd.Name) == nil {
			t.Errorf("Doc().Flags contém %q, mas Command().Flags() não tem essa flag", fd.Name)
		}
	}
}

func TestExpectedFlagsExist(t *testing.T) {
	cmd := New().Command()

	cases := []struct {
		name      string
		shorthand string
	}{
		{"input", "i"},
		{"output-dir", "o"},
		{"mode", ""},
		{"ranges", ""},
		{"regex", ""},
		{"name-template", ""},
		{"overwrite", ""},
		{"ocr", ""},
		{"ocr-lang", ""},
	}

	for _, c := range cases {
		f := cmd.Flags().Lookup(c.name)
		if f == nil {
			t.Fatalf("flag %q não encontrada", c.name)
		}
		if f.Shorthand != c.shorthand {
			t.Errorf("flag %q: shorthand = %q, want %q", c.name, f.Shorthand, c.shorthand)
		}
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()

	if opts.Mode != "page" {
		t.Errorf("defaultOptions().Mode = %q, want %q", opts.Mode, "page")
	}
	if opts.NameTemplate != "pagina-%03d" {
		t.Errorf("defaultOptions().NameTemplate = %q, want %q", opts.NameTemplate, "pagina-%03d")
	}
	if opts.OCR != "auto" {
		t.Errorf("defaultOptions().OCR = %q, want %q", opts.OCR, "auto")
	}
	if opts.OCRLang != "por" {
		t.Errorf("defaultOptions().OCRLang = %q, want %q", opts.OCRLang, "por")
	}
}

func TestRunEmptyOptionsErrorsWithoutPanic(t *testing.T) {
	tl := New()
	tl.opts = Options{}

	if _, err := tl.run(); err == nil {
		t.Fatal("run() com Options vazias deveria devolver erro (input faltando)")
	}
}

func TestRunInvalidModeErrors(t *testing.T) {
	tl := New()
	tl.opts = Options{
		Input: "arquivo.pdf",
		Mode:  "xyz",
	}

	_, err := tl.run()
	if err == nil {
		t.Fatal("run() com Mode inválido deveria devolver erro")
	}
	if !strings.Contains(err.Error(), "page") || !strings.Contains(err.Error(), "range") || !strings.Contains(err.Error(), "regex") {
		t.Errorf("erro %q deveria mencionar os modos válidos (page, range, regex)", err.Error())
	}
}

func TestRunRegexModeEmptyRegexErrors(t *testing.T) {
	tl := New()
	tl.opts = Options{
		Input: "arquivo.pdf",
		Mode:  "regex",
		Regex: "",
	}

	if _, err := tl.run(); err == nil {
		t.Fatal("run() com Mode regex e Regex vazia deveria devolver erro")
	}
}

func TestRunRegexModeInvalidRegexErrorsWithoutPanic(t *testing.T) {
	tl := New()
	tl.opts = Options{
		Input: "arquivo.pdf",
		Mode:  "regex",
		Regex: "[",
	}

	if _, err := tl.run(); err == nil {
		t.Fatal("run() com Regex inválida (\"[\") deveria devolver erro de compilação")
	}
}

func TestRunRangeModeEmptyRangesErrors(t *testing.T) {
	tl := New()
	tl.opts = Options{
		Input:  "arquivo.pdf",
		Mode:   "range",
		Ranges: "",
	}

	if _, err := tl.run(); err == nil {
		t.Fatal("run() com Mode range e Ranges vazio deveria devolver erro")
	}
}

func TestRunInvalidOCRModeErrors(t *testing.T) {
	tl := New()
	tl.opts = Options{
		Input: "arquivo.pdf",
		Mode:  "page",
		OCR:   "xyz",
	}

	_, err := tl.run()
	if err == nil {
		t.Fatal("run() com OCR inválido deveria devolver erro")
	}
	if !strings.Contains(err.Error(), "auto") || !strings.Contains(err.Error(), "always") || !strings.Contains(err.Error(), "never") {
		t.Errorf("erro %q deveria mencionar os valores válidos de OCR (auto, always, never)", err.Error())
	}
}

func TestTextOptionsNeverHasNoEngine(t *testing.T) {
	tl := New()
	tl.opts = Options{OCR: "never", OCRLang: "por"}

	text, err := tl.textOptions()
	if err != nil {
		t.Fatalf("textOptions() erro inesperado: %v", err)
	}
	if text.Mode != pdfutil.OCRNever {
		t.Errorf("textOptions().Mode = %q, want %q", text.Mode, pdfutil.OCRNever)
	}
	if text.Engine != nil {
		t.Errorf("textOptions().Engine = %v, want nil quando OCR = never", text.Engine)
	}
}

func TestTextOptionsAutoHasEngine(t *testing.T) {
	tl := New()
	tl.opts = Options{OCR: "auto", OCRLang: "por"}

	text, err := tl.textOptions()
	if err != nil {
		t.Fatalf("textOptions() erro inesperado: %v", err)
	}
	if text.Mode != pdfutil.OCRAuto {
		t.Errorf("textOptions().Mode = %q, want %q", text.Mode, pdfutil.OCRAuto)
	}
	if text.Engine == nil {
		t.Error("textOptions().Engine = nil, want não-nil quando OCR = auto")
	}
}

func TestProfileNotNil(t *testing.T) {
	tl := New()
	if tl.Profile() == nil {
		t.Fatal("Profile() não deveria ser nil")
	}
}

func TestProfileApplyWrongTypeErrorsWithoutPanic(t *testing.T) {
	tl := New()
	p := tl.Profile()

	_, err := p.Apply("not-an-options-pointer")
	if err == nil {
		t.Fatal("Apply() com tipo errado deveria devolver erro, não panic")
	}
}

func TestProfileEmpty(t *testing.T) {
	tl := New()
	p := tl.Profile()

	empty, ok := p.Empty().(*Options)
	if !ok {
		t.Fatalf("Empty() devolveu tipo %T, esperava *Options", p.Empty())
	}
	if empty.Mode != "page" {
		t.Errorf("Empty().Mode = %q, want %q", empty.Mode, "page")
	}
}

// TestModeCompletion garante que a completação de --mode devolve
// exatamente os três modos aceitos, na ordem estável de modeOrder, sem
// completar arquivo.
func TestModeCompletion(t *testing.T) {
	cmd := New().Command()

	fn, ok := cmd.GetFlagCompletionFunc("mode")
	if !ok {
		t.Fatal("nenhuma função de completação registrada para --mode")
	}

	got, directive := fn(cmd, nil, "")

	want := []string{"page", "range", "regex"}
	if len(got) != len(want) {
		t.Fatalf("completação de --mode = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("completação de --mode[%d] = %q, want %q", i, got[i], w)
		}
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
}

// TestOCRCompletion garante que a completação de --ocr devolve exatamente
// os três valores aceitos, sem completar arquivo.
func TestOCRCompletion(t *testing.T) {
	cmd := New().Command()

	fn, ok := cmd.GetFlagCompletionFunc("ocr")
	if !ok {
		t.Fatal("nenhuma função de completação registrada para --ocr")
	}

	got, directive := fn(cmd, nil, "")

	want := []string{"auto", "always", "never"}
	if len(got) != len(want) {
		t.Fatalf("completação de --ocr = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("completação de --ocr[%d] = %q, want %q", i, got[i], w)
		}
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
}

// TestOCRLangCompletionFallsBackWithoutTesseract prova que, sem o
// tesseract disponível (PATH e TESSERACT_PATH isolados, como faz
// e2e/helpers_test.go), a completação de --ocr-lang devolve a lista fixa
// conhecida (por, eng) em vez de travar ou falhar.
func TestOCRLangCompletionFallsBackWithoutTesseract(t *testing.T) {
	t.Setenv("PATH", "/nao-existe/bin")
	t.Setenv("TESSERACT_PATH", "/nao-existe/tesseract")

	cmd := New().Command()

	fn, ok := cmd.GetFlagCompletionFunc("ocr-lang")
	if !ok {
		t.Fatal("nenhuma função de completação registrada para --ocr-lang")
	}

	got, directive := fn(cmd, nil, "")

	if len(got) != 2 {
		t.Fatalf("completação de --ocr-lang sem tesseract = %v, esperava 2 entradas (por, eng)", got)
	}
	if !strings.HasPrefix(got[0], "por\t") || !strings.HasPrefix(got[1], "eng\t") {
		t.Errorf("completação de --ocr-lang sem tesseract = %v, esperava prefixos \"por\\t\" e \"eng\\t\"", got)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
}
