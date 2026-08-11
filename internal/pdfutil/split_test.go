package pdfutil

import (
	"regexp"
	"strings"
	"testing"
)

func TestSanitizeFilename(t *testing.T) {
	cases := []string{
		"../../etc/passwd",
		"a/b",
		"CON",
		"nome: com | inválidos",
		"  espaços  ",
		"",
	}

	for _, in := range cases {
		got := SanitizeFilename(in)
		if strings.Contains(got, "/") || strings.Contains(got, "\\") {
			t.Errorf("SanitizeFilename(%q) = %q contém separador de caminho", in, got)
		}
		if strings.Contains(got, "..") {
			t.Errorf("SanitizeFilename(%q) = %q contém \"..\"", in, got)
		}
	}

	if got := SanitizeFilename("  espaços  "); got != "espaços" {
		t.Errorf("SanitizeFilename(espaços com trim) = %q, esperava %q", got, "espaços")
	}

	if got := SanitizeFilename(""); got != "" {
		t.Errorf("SanitizeFilename(\"\") = %q, esperava vazio", got)
	}
}

func TestParseRangesValid(t *testing.T) {
	got, err := ParseRanges("1-5, 6-10 ,11-,3,-4")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	want := []string{"1-5", "6-10", "11-", "3", "-4"}
	if len(got) != len(want) {
		t.Fatalf("esperava %v, obteve %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("esperava %v, obteve %v", want, got)
		}
	}
}

func TestParseRangesIgnoresEmptyItems(t *testing.T) {
	got, err := ParseRanges("1-5,,  ,6-10")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("esperava 2 itens ignorando vazios, obteve %v", got)
	}
}

func TestParseRangesInvalid(t *testing.T) {
	invalidSpecs := []string{"abc", "1-2-3", "1--2", "-", "a-1"}
	for _, spec := range invalidSpecs {
		if _, err := ParseRanges(spec); err == nil {
			t.Errorf("ParseRanges(%q) deveria falhar", spec)
		}
	}
}

func TestGroupPagesByRegexEmpty(t *testing.T) {
	got := GroupPagesByRegex(nil, regexp.MustCompile(`x`), "")
	if len(got) != 0 {
		t.Fatalf("esperava slice vazio para pageTexts vazio, obteve %v", got)
	}
}

func TestGroupPagesByRegexAllMatch(t *testing.T) {
	pages := []string{"Nota 1", "Nota 2", "Nota 3"}
	re := regexp.MustCompile(`Nota (\d+)`)

	got := GroupPagesByRegex(pages, re, "")
	if len(got) != 3 {
		t.Fatalf("esperava 3 grupos (1 por página), obteve %d: %v", len(got), got)
	}
	for i, g := range got {
		if g.Start != i+1 || g.End != i+1 {
			t.Errorf("grupo %d: esperava Start=End=%d, obteve Start=%d End=%d", i, i+1, g.Start, g.End)
		}
	}
	if got[0].Name != "1" || got[1].Name != "2" || got[2].Name != "3" {
		t.Errorf("nomes inesperados: %+v", got)
	}
}

func TestGroupPagesByRegexNoMatch(t *testing.T) {
	pages := []string{"texto qualquer", "outro texto", "mais texto"}
	re := regexp.MustCompile(`NUNCA-VAI-CASAR`)

	got := GroupPagesByRegex(pages, re, "")
	if len(got) != 1 {
		t.Fatalf("esperava 1 grupo com tudo quando nada casa, obteve %d: %v", len(got), got)
	}
	if got[0].Start != 1 || got[0].End != 3 {
		t.Fatalf("esperava grupo cobrindo páginas 1-3, obteve Start=%d End=%d", got[0].Start, got[0].End)
	}
}

func TestGroupPagesByRegexLeadingUnmatchedPages(t *testing.T) {
	pages := []string{"capa sem identificação", "Nota 100", "conteúdo da nota 100"}
	re := regexp.MustCompile(`Nota (\d+)`)

	got := GroupPagesByRegex(pages, re, "")
	if len(got) != 2 {
		t.Fatalf("esperava 2 grupos (capa + nota), obteve %d: %v", len(got), got)
	}
	if got[0].Start != 1 || got[0].End != 1 {
		t.Fatalf("primeiro grupo deveria cobrir só a página 1, obteve Start=%d End=%d", got[0].Start, got[0].End)
	}
	if got[1].Start != 2 || got[1].End != 3 {
		t.Fatalf("segundo grupo deveria cobrir páginas 2-3, obteve Start=%d End=%d", got[1].Start, got[1].End)
	}
	if got[1].Name != "100" {
		t.Fatalf("esperava nome capturado \"100\", obteve %q", got[1].Name)
	}
}

func TestGroupPagesByRegexCaptureNaming(t *testing.T) {
	pages := []string{"Nota Fiscal 4521"}
	re := regexp.MustCompile(`Nota Fiscal (\d+)`)

	got := GroupPagesByRegex(pages, re, "")
	if len(got) != 1 || got[0].Name != "4521" {
		t.Fatalf("esperava grupo nomeado \"4521\", obteve %+v", got)
	}
}

func TestGroupPagesByRegexTemplateNamingWithoutCapture(t *testing.T) {
	pages := []string{"documento A", "documento B"}
	re := regexp.MustCompile(`documento`)

	got := GroupPagesByRegex(pages, re, "arquivo-%03d")
	if len(got) != 2 {
		t.Fatalf("esperava 2 grupos, obteve %d", len(got))
	}
	if got[0].Name != "arquivo-001" || got[1].Name != "arquivo-002" {
		t.Fatalf("nomes de template inesperados: %+v", got)
	}
}

func TestGroupPagesByRegexDuplicateNamesDisambiguated(t *testing.T) {
	pages := []string{"Nota 7", "conteúdo", "Nota 7", "outro conteúdo"}
	re := regexp.MustCompile(`Nota (\d+)`)

	got := GroupPagesByRegex(pages, re, "")
	if len(got) != 2 {
		t.Fatalf("esperava 2 grupos, obteve %d: %v", len(got), got)
	}
	if got[0].Name != "7" {
		t.Fatalf("primeiro grupo deveria se chamar \"7\", obteve %q", got[0].Name)
	}
	if got[1].Name != "7-2" {
		t.Fatalf("segundo grupo duplicado deveria se chamar \"7-2\", obteve %q", got[1].Name)
	}
}

func TestGroupPagesByRegexEmptyCaptureFallsBackToTemplate(t *testing.T) {
	pages := []string{"Nota ", "conteúdo"}
	re := regexp.MustCompile(`Nota (\d*)`)

	got := GroupPagesByRegex(pages, re, "documento-%03d")
	if len(got) != 1 {
		t.Fatalf("esperava 1 grupo, obteve %d: %v", len(got), got)
	}
	if got[0].Name != "documento-001" {
		t.Fatalf("captura vazia deveria cair no template, obteve %q", got[0].Name)
	}
}
