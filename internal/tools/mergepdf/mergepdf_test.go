package mergepdf

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestNewNotNil(t *testing.T) {
	if New() == nil {
		t.Fatal("New() devolveu nil")
	}
}

func TestMetaID(t *testing.T) {
	got := New().Meta().ID
	want := "merge-pdf"
	if got != want {
		t.Fatalf("Meta().ID = %q, want %q", got, want)
	}
}

func TestCommandUse(t *testing.T) {
	use := New().Command().Use
	want := "merge-pdf"
	if use != want {
		t.Fatalf("Command().Use = %q, want %q", use, want)
	}
}

// TestEmptyInputsAdvice_RetriesBeforeLimit cobre o caso comum: ainda há
// tentativas sobrando, então o laço de escolha de entradas (pickInputs) não
// deve desistir. Mesmo defeito e mesma correção que ocr-pdf (ver
// internal/tools/ocrpdf/ocrpdf_test.go).
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
// que desistir para nunca virar um laço infinito.
func TestEmptyInputsAdvice_GivesUpAtLimit(t *testing.T) {
	message, giveUp := emptyInputsAdvice(maxEmptyInputsAttempts, maxEmptyInputsAttempts)

	if !giveUp {
		t.Fatalf("emptyInputsAdvice(%d, %d) deveria desistir no limite de tentativas", maxEmptyInputsAttempts, maxEmptyInputsAttempts)
	}
	if !strings.Contains(message, "limite") {
		t.Errorf("mensagem %q não menciona o limite de tentativas", message)
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

	for _, f := range doc.Flags {
		if cmd.Flags().Lookup(f.Name) == nil {
			t.Errorf("Doc().Flags contém a flag %q, mas ela não existe em Command().Flags()", f.Name)
		}
	}
}

func TestCommandFlags(t *testing.T) {
	cmd := New().Command()
	fs := cmd.Flags()

	cases := []struct {
		name      string
		shorthand string
	}{
		{"input", "i"},
		{"max-depth", ""},
		{"output", "o"},
		{"sort", ""},
		{"overwrite", ""},
	}

	for _, c := range cases {
		f := fs.Lookup(c.name)
		if f == nil {
			t.Fatalf("flag %q não encontrada em Command().Flags()", c.name)
		}
		if f.Shorthand != c.shorthand {
			t.Errorf("flag %q: shorthand = %q, want %q", c.name, f.Shorthand, c.shorthand)
		}
	}
}

func TestDefaultOptions(t *testing.T) {
	got := defaultOptions()
	if got.MaxDepth != 1 {
		t.Errorf("defaultOptions().MaxDepth = %d, want 1", got.MaxDepth)
	}
	if got.Sort != "name" {
		t.Errorf("defaultOptions().Sort = %q, want %q", got.Sort, "name")
	}
}

func TestRunValidatesEmptyOptions(t *testing.T) {
	tl := New()
	tl.opts = Options{}

	if _, err := tl.run(); err == nil {
		t.Fatal("run() com Options vazias deveria devolver erro, devolveu nil")
	}
}

func TestProfileNotNil(t *testing.T) {
	if New().Profile() == nil {
		t.Fatal("Profile() devolveu nil")
	}
}

func TestProfileEmpty(t *testing.T) {
	empty := New().Profile().Empty()
	opts, ok := empty.(*Options)
	if !ok {
		t.Fatalf("Empty() devolveu %T, want *Options", empty)
	}
	if opts.MaxDepth != 1 || opts.Sort != "name" {
		t.Errorf("Empty() = %+v, want defaults aplicados", opts)
	}
}

func TestProfileApplyWrongTypeReturnsError(t *testing.T) {
	_, err := New().Profile().Apply("não é *Options")
	if err == nil {
		t.Fatal("Apply() com tipo errado deveria devolver erro, devolveu nil")
	}
}

// TestSortCompletion garante que a completação de --sort devolve
// exatamente os dois valores aceitos ("name", "mtime"), sem completar
// arquivo (ShellCompDirectiveNoFileComp).
func TestSortCompletion(t *testing.T) {
	cmd := New().Command()

	fn, ok := cmd.GetFlagCompletionFunc("sort")
	if !ok {
		t.Fatal("nenhuma função de completação registrada para --sort")
	}

	got, directive := fn(cmd, nil, "")

	want := []string{"name", "mtime"}
	if len(got) != len(want) {
		t.Fatalf("completação de --sort = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("completação de --sort[%d] = %q, want %q", i, got[i], w)
		}
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
}
