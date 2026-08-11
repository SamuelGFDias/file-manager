package ocrpdf

import (
	"strings"
	"testing"
)

func TestMetaID(t *testing.T) {
	got := New().Meta().ID
	want := "ocr-pdf"
	if got != want {
		t.Fatalf("Meta().ID = %q, want %q", got, want)
	}
}

func TestCommandUse(t *testing.T) {
	use := New().Command().Use
	want := "ocr-pdf"
	if !strings.HasPrefix(use, want) {
		t.Fatalf("Command().Use = %q, want prefix %q", use, want)
	}
}

func TestDocFlagsMatchesParams(t *testing.T) {
	tl := New()

	doc := tl.Doc()
	params := tl.params()

	if len(doc.Flags) != len(params) {
		t.Fatalf("Doc().Flags tem %d entradas, esperava %d (mesmo tamanho de params())", len(doc.Flags), len(params))
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()
	if opts.MaxDepth != 1 {
		t.Errorf("MaxDepth padrão = %d, esperava 1", opts.MaxDepth)
	}
	if opts.Suffix != "-ocr" {
		t.Errorf("Suffix padrão = %q, esperava \"-ocr\"", opts.Suffix)
	}
	if opts.Lang != "por" {
		t.Errorf("Lang padrão = %q, esperava \"por\"", opts.Lang)
	}
	if opts.Overwrite || opts.SkipExisting || opts.DryRun {
		t.Error("flags booleanas deveriam começar false")
	}
}

// TestRunWithoutInputFails confere que run() exige --input antes de
// qualquer outra validação (inclusive antes de checar o Tesseract) — um
// erro de uso claro, sem depender do ambiente ter (ou não) o motor de OCR
// instalado.
func TestRunWithoutInputFails(t *testing.T) {
	tl := New()
	_, err := tl.run()
	if err == nil {
		t.Fatal("esperava erro sem nenhum --input informado")
	}
	if !strings.Contains(err.Error(), "input") {
		t.Errorf("erro = %q, esperava mencionar --input", err.Error())
	}
}

// TestFormatProgressLine confere o formato exato pedido para a linha de
// progresso ("[3/120] nota-003.pdf — 2 página(s)...") — um arquivo
// inexistente devolve 0 páginas em vez de propagar erro, já que a linha de
// progresso nunca pode interromper o processamento por causa de uma falha
// acessória (contagem de páginas).
func TestFormatProgressLine(t *testing.T) {
	got := formatProgressLine(3, 120, "/tmp/nao-existe-de-verdade.pdf")
	want := "[3/120] nao-existe-de-verdade.pdf — 0 página(s)..."
	if got != want {
		t.Fatalf("formatProgressLine = %q, esperava %q", got, want)
	}
}

// TestEmptyInputsAdvice_RetriesBeforeLimit cobre o caso comum: ainda há
// tentativas sobrando, então o laço de escolha de entradas (pickInputs) não
// deve desistir.
func TestEmptyInputsAdvice_RetriesBeforeLimit(t *testing.T) {
	message, giveUp := emptyInputsAdvice(1, maxEmptyInputsAttempts)

	if giveUp {
		t.Fatalf("emptyInputsAdvice(1, %d) desistiu antes do limite", maxEmptyInputsAttempts)
	}
	if !strings.Contains(message, "nenhum arquivo ou pasta") {
		t.Errorf("mensagem %q não menciona a ausência de entradas", message)
	}
}

// TestEmptyInputsAdvice_GivesUpAtLimit cobre a última tentativa: o laço tem
// que desistir para nunca virar um laço infinito, e a mensagem final deve
// deixar claro que o limite foi atingido.
func TestEmptyInputsAdvice_GivesUpAtLimit(t *testing.T) {
	message, giveUp := emptyInputsAdvice(maxEmptyInputsAttempts, maxEmptyInputsAttempts)

	if !giveUp {
		t.Fatalf("emptyInputsAdvice(%d, %d) deveria desistir no limite de tentativas", maxEmptyInputsAttempts, maxEmptyInputsAttempts)
	}
	if !strings.Contains(message, "limite") {
		t.Errorf("mensagem %q não menciona o limite de tentativas", message)
	}
}

func TestProfileEmptyReturnsDefaults(t *testing.T) {
	tl := New()
	profile := tl.Profile()
	if profile == nil {
		t.Fatal("Profile() não deveria ser nil: ocr-pdf suporta perfis salvos")
	}

	empty, ok := profile.Empty().(*Options)
	if !ok {
		t.Fatalf("Empty() devolveu tipo %T, esperava *Options", profile.Empty())
	}
	if empty.Suffix != "-ocr" {
		t.Errorf("Empty().Suffix = %q, esperava \"-ocr\"", empty.Suffix)
	}
}

func TestApplyRejectsWrongType(t *testing.T) {
	tl := New()
	profile := tl.Profile()
	_, err := profile.Apply("tipo errado")
	if err == nil {
		t.Fatal("esperava erro ao aplicar um perfil com tipo inválido")
	}
}
