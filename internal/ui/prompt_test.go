package ui

import (
	"strings"
	"testing"
	"text/template"

	"github.com/AlecAivazis/survey/v2"
)

// TestApplyThemeIdempotent garante que chamar ApplyTheme() mais de uma vez
// não entra em pânico e que o template instalado é sempre o mesmo — é isso
// que permite chamá-la sem custo extra a cada NewScreen do menu principal.
func TestApplyThemeIdempotent(t *testing.T) {
	ApplyTheme()
	firstSelect := survey.SelectQuestionTemplate
	firstMultiSelect := survey.MultiSelectQuestionTemplate

	ApplyTheme()
	secondSelect := survey.SelectQuestionTemplate
	secondMultiSelect := survey.MultiSelectQuestionTemplate

	if firstSelect != secondSelect {
		t.Fatalf("ApplyTheme() não é idempotente: template de Select mudou entre chamadas")
	}
	if firstMultiSelect != secondMultiSelect {
		t.Fatalf("ApplyTheme() não é idempotente: template de MultiSelect mudou entre chamadas")
	}
}

// TestApplyThemeInstallsDescriptionOnlyOnSelected prova que o tema
// realmente foi aplicado: a condição da descrição agora exige que a opção
// seja a selecionada, e a dica em inglês original não está mais presente.
func TestApplyThemeInstallsDescriptionOnlyOnSelected(t *testing.T) {
	ApplyTheme()

	tmpl := survey.SelectQuestionTemplate

	if !strings.Contains(tmpl, "eq .SelectedIndex .CurrentIndex") {
		t.Fatalf("template não contém a condição de opção selecionada (eq .SelectedIndex .CurrentIndex): %q", tmpl)
	}

	if strings.Contains(tmpl, "Use arrows to move") {
		t.Fatalf("template ainda contém a dica em inglês \"Use arrows to move\": %q", tmpl)
	}
}

// TestApplyThemeTemplateCompiles garante que o template instalado é um
// text/template válido. Um template quebrado só se manifestaria em tempo de
// execução, na cara do usuário — este teste é a única rede de proteção
// contra isso.
func TestApplyThemeTemplateCompiles(t *testing.T) {
	ApplyTheme()

	// As funções customizadas do survey (ex: "color") não importam para
	// validar a sintaxe do template; registramos versões no-op só para que
	// o parse não falhe por função desconhecida.
	noopFuncs := template.FuncMap{
		"color": func(string) string { return "" },
	}

	if _, err := template.New("select-template-test").Funcs(noopFuncs).Parse(survey.SelectQuestionTemplate); err != nil {
		t.Fatalf("template instalado por ApplyTheme() não compila: %v", err)
	}
}

// TestApplyThemeInstallsSpaceHintOnMultiSelect prova que o template de
// MultiSelect foi de fato traduzido: a dica em português menciona a barra
// de espaço (o ponto central do defeito relatado — o usuário não sabia que
// a marcação é feita com espaço, não com Enter), e a dica original em
// inglês não está mais presente.
func TestApplyThemeInstallsSpaceHintOnMultiSelect(t *testing.T) {
	ApplyTheme()

	tmpl := survey.MultiSelectQuestionTemplate

	if !strings.Contains(tmpl, "ESPAÇO") {
		t.Fatalf("template de MultiSelect não menciona a barra de espaço: %q", tmpl)
	}

	if strings.Contains(tmpl, "space to select") {
		t.Fatalf("template de MultiSelect ainda contém a dica em inglês \"space to select\": %q", tmpl)
	}
}

// TestApplyThemeMultiSelectTemplateCompiles garante que o template de
// MultiSelect instalado por ApplyTheme() é um text/template válido — mesma
// rede de proteção que TestApplyThemeTemplateCompiles já dá ao template de
// Select.
func TestApplyThemeMultiSelectTemplateCompiles(t *testing.T) {
	ApplyTheme()

	noopFuncs := template.FuncMap{
		"color": func(string) string { return "" },
	}

	if _, err := template.New("multiselect-template-test").Funcs(noopFuncs).Parse(survey.MultiSelectQuestionTemplate); err != nil {
		t.Fatalf("template de MultiSelect instalado por ApplyTheme() não compila: %v", err)
	}
}

func TestBoldEmptyStringDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Bold(\"\") entrou em pânico: %v", r)
		}
	}()
	_ = Bold("")
}

func TestHighlightEmptyStringDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Highlight(\"\") entrou em pânico: %v", r)
		}
	}()
	_ = Highlight("")
}

func TestPathTextEmptyStringDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PathText(\"\") entrou em pânico: %v", r)
		}
	}()
	_ = PathText("")
}

// TestHelpersPreserveOriginalText garante que os helpers de formatação não
// removem o texto original — o fatih/color desliga a cor automaticamente
// quando a saída não é um terminal (como em testes), então aqui verificamos
// o CONTEÚDO, nunca a presença de códigos ANSI: um teste que exigisse ANSI
// passaria localmente num terminal e falharia em CI (ou o oposto).
func TestHelpersPreserveOriginalText(t *testing.T) {
	const text = "algum texto"

	if got := Bold(text); !strings.Contains(got, text) {
		t.Errorf("Bold(%q) = %q, não contém o texto original", text, got)
	}
	if got := Highlight(text); !strings.Contains(got, text) {
		t.Errorf("Highlight(%q) = %q, não contém o texto original", text, got)
	}
	if got := PathText(text); !strings.Contains(got, text) {
		t.Errorf("PathText(%q) = %q, não contém o texto original", text, got)
	}
}

func TestCountSingular(t *testing.T) {
	got := Count(1, "PDF", "PDFs")
	if !strings.Contains(got, "1 PDF") {
		t.Errorf("Count(1, \"PDF\", \"PDFs\") = %q, esperava conter \"1 PDF\"", got)
	}
	if strings.Contains(got, "1 PDFs") {
		t.Errorf("Count(1, \"PDF\", \"PDFs\") = %q, não deveria usar o plural", got)
	}
}

func TestCountPlural(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "0 PDFs"},
		{2, "2 PDFs"},
		{12, "12 PDFs"},
	}

	for _, c := range cases {
		got := Count(c.n, "PDF", "PDFs")
		if !strings.Contains(got, c.want) {
			t.Errorf("Count(%d, \"PDF\", \"PDFs\") = %q, esperava conter %q", c.n, got, c.want)
		}
	}
}
