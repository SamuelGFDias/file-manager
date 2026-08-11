package organizepdf

import (
	"strings"
	"testing"
)

func TestParseLevelFlagsBasic(t *testing.T) {
	specs, err := ParseLevelFlags([]string{`fornecedor=FORNECEDOR:\s*(\w+)`})
	if err != nil {
		t.Fatalf("ParseLevelFlags() erro inesperado: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("ParseLevelFlags() devolveu %d specs, esperava 1", len(specs))
	}
	if specs[0].Label != "fornecedor" {
		t.Errorf("specs[0].Label = %q, want %q", specs[0].Label, "fornecedor")
	}
	if specs[0].Regex != `FORNECEDOR:\s*(\w+)` {
		t.Errorf("specs[0].Regex = %q, want %q", specs[0].Regex, `FORNECEDOR:\s*(\w+)`)
	}
}

// TestParseLevelFlagsRegexContainingEquals é o teste crítico: a regex do
// lado direito do "=" quase sempre contém "=" (ex: em asserções como
// "TOTAL\s*=\s*(\d+)"); dividir em TODAS as ocorrências de "=" quebraria a
// regex ao meio. ParseLevelFlags deve dividir só no primeiro "=".
func TestParseLevelFlagsRegexContainingEquals(t *testing.T) {
	specs, err := ParseLevelFlags([]string{`total=TOTAL\s*=\s*(\d+)`})
	if err != nil {
		t.Fatalf("ParseLevelFlags() erro inesperado: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("ParseLevelFlags() devolveu %d specs, esperava 1", len(specs))
	}
	if specs[0].Label != "total" {
		t.Errorf("specs[0].Label = %q, want %q", specs[0].Label, "total")
	}
	if specs[0].Regex != `TOTAL\s*=\s*(\d+)` {
		t.Errorf("specs[0].Regex = %q, want %q (regex inteira preservada, dividida só no primeiro '=')", specs[0].Regex, `TOTAL\s*=\s*(\d+)`)
	}
}

func TestParseLevelFlagsEmptyLabelErrors(t *testing.T) {
	if _, err := ParseLevelFlags([]string{"=abc"}); err == nil {
		t.Fatal("ParseLevelFlags() com rótulo vazio deveria devolver erro")
	}
}

func TestParseLevelFlagsEmptyRegexErrors(t *testing.T) {
	if _, err := ParseLevelFlags([]string{"abc="}); err == nil {
		t.Fatal("ParseLevelFlags() com regex vazia deveria devolver erro")
	}
}

func TestParseLevelFlagsMissingEqualsErrors(t *testing.T) {
	if _, err := ParseLevelFlags([]string{"semigual"}); err == nil {
		t.Fatal("ParseLevelFlags() sem '=' deveria devolver erro")
	}
}

func TestParseLevelFlagsNilInputReturnsEmptySlice(t *testing.T) {
	specs, err := ParseLevelFlags(nil)
	if err != nil {
		t.Fatalf("ParseLevelFlags(nil) erro inesperado: %v", err)
	}
	if len(specs) != 0 {
		t.Fatalf("ParseLevelFlags(nil) devolveu %d specs, esperava 0 (modo somente renomear)", len(specs))
	}
}

func TestBuildLevelsValidSpecsCompile(t *testing.T) {
	levels, err := BuildLevels([]LevelSpec{
		{Label: "fornecedor", Regex: `FORNECEDOR:\s*(\w+)`},
		{Label: "filial", Regex: `FILIAL\s*(\d+)`},
	})
	if err != nil {
		t.Fatalf("BuildLevels() erro inesperado: %v", err)
	}
	if len(levels) != 2 {
		t.Fatalf("BuildLevels() devolveu %d levels, esperava 2", len(levels))
	}
	if levels[0].Label != "fornecedor" || levels[1].Label != "filial" {
		t.Errorf("BuildLevels() não preservou a ordem/rótulos: %+v", levels)
	}
}

func TestBuildLevelsInvalidRegexMentionsLabel(t *testing.T) {
	_, err := BuildLevels([]LevelSpec{
		{Label: "fornecedor", Regex: `FORNECEDOR:\s*(\w+)`},
		{Label: "filial-quebrada", Regex: "["},
	})
	if err == nil {
		t.Fatal("BuildLevels() com regex inválida deveria devolver erro")
	}
	if !strings.Contains(err.Error(), "filial-quebrada") {
		t.Errorf("erro %q deveria mencionar o rótulo do nível problemático (%q)", err.Error(), "filial-quebrada")
	}
}

func TestBuildLevelsEmptySliceReturnsEmptySlice(t *testing.T) {
	levels, err := BuildLevels(nil)
	if err != nil {
		t.Fatalf("BuildLevels(nil) erro inesperado: %v", err)
	}
	if len(levels) != 0 {
		t.Fatalf("BuildLevels(nil) devolveu %d levels, esperava 0", len(levels))
	}
}

func TestMetaID(t *testing.T) {
	got := New().Meta().ID
	want := "organize-pdf"
	if got != want {
		t.Fatalf("Meta().ID = %q, want %q", got, want)
	}
}

func TestCommandUse(t *testing.T) {
	got := New().Command().Use
	want := "organize-pdf"
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
		{"output", "o"},
		{"level", ""},
		{"filename-regex", ""},
		{"move", ""},
		{"unclassified-dir", ""},
		{"overwrite", ""},
		{"dry-run", ""},
		{"sample", ""},
	}

	if len(cases) != 9 {
		t.Fatalf("teste declara %d flags, esperava 9", len(cases))
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

	if opts.UnclassifiedDir != "sem-classificacao" {
		t.Errorf("defaultOptions().UnclassifiedDir = %q, want %q", opts.UnclassifiedDir, "sem-classificacao")
	}
	if opts.Move != false {
		t.Error("defaultOptions().Move deveria ser false — o padrão é copiar (não destrutivo); se isso falhou, a ferramenta ficou destrutiva por padrão")
	}
}

func TestRunEmptyOptionsErrorsWithoutPanic(t *testing.T) {
	tl := New()
	tl.opts = Options{}

	if _, err := tl.run(); err == nil {
		t.Fatal("run() com Options vazias deveria devolver erro (input/output faltando)")
	}
}

func TestRunInvalidFilenameRegexErrorsWithoutPanic(t *testing.T) {
	tl := New()
	tl.opts = Options{
		InputDir:      "entrada",
		OutputDir:     "saida",
		FilenameRegex: "[",
	}

	_, err := tl.run()
	if err == nil {
		t.Fatal("run() com FilenameRegex inválida (\"[\") deveria devolver erro de compilação")
	}
}

func TestRunInvalidLevelRegexErrorsWithoutPanic(t *testing.T) {
	tl := New()
	tl.opts = Options{
		InputDir:  "entrada",
		OutputDir: "saida",
		Levels:    []LevelSpec{{Label: "fornecedor", Regex: "["}},
	}

	_, err := tl.run()
	if err == nil {
		t.Fatal("run() com regex de nível inválida deveria devolver erro")
	}
	if !strings.Contains(err.Error(), "fornecedor") {
		t.Errorf("erro %q deveria mencionar o rótulo do nível problemático", err.Error())
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
	if empty.UnclassifiedDir != "sem-classificacao" {
		t.Errorf("Empty().UnclassifiedDir = %q, want %q", empty.UnclassifiedDir, "sem-classificacao")
	}
	if empty.Move != false {
		t.Error("Empty().Move deveria ser false")
	}
}
