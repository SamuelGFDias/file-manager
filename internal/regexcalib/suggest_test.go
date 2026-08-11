package regexcalib

import (
	"regexp"
	"testing"
	"time"
	"unicode/utf8"
)

func TestSuggest_BasicSingleOccurrence(t *testing.T) {
	text := "NOTA FISCAL\nNF: 12345\nEmitida em 01/01/2026"
	value := "12345"

	candidates := Suggest(text, value)
	if len(candidates) != 1 {
		t.Fatalf("esperava 1 candidato, obteve %d: %+v", len(candidates), candidates)
	}

	c := candidates[0]

	captured, ok, err := Preview(c.Pattern, text)
	if err != nil {
		t.Fatalf("Preview retornou erro inesperado: %v", err)
	}
	if !ok {
		t.Fatalf("Preview não casou com o texto original; pattern=%q", c.Pattern)
	}
	if captured != value {
		t.Fatalf("Preview capturou %q, esperava %q", captured, value)
	}

	variation := "NF:    12345"
	capturedVar, okVar, errVar := Preview(c.Pattern, variation)
	if errVar != nil {
		t.Fatalf("Preview (variação de espaço) retornou erro inesperado: %v", errVar)
	}
	if !okVar {
		t.Fatalf("Preview não casou com a variação de espaçamento; pattern=%q", c.Pattern)
	}
	if capturedVar != value {
		t.Fatalf("Preview (variação) capturou %q, esperava %q", capturedVar, value)
	}
}

func TestSuggest_MultipleOccurrences(t *testing.T) {
	text := "Relatório de vendas do mês corrente\n" +
		"Pedido: 999 foi registrado com sucesso pela manhã\n" +
		"Depois de processado, o item seguiu para conferência e separação\n" +
		"Total do pedido: 999 unidades vendidas ao cliente final"
	value := "999"

	candidates := Suggest(text, value)
	if len(candidates) < 2 {
		t.Fatalf("esperava pelo menos 2 candidatos, obteve %d: %+v", len(candidates), candidates)
	}

	if candidates[0].Index == candidates[1].Index {
		t.Fatalf("candidatos deveriam ter Index diferentes, ambos são %d", candidates[0].Index)
	}
	if candidates[0].Context == candidates[1].Context {
		t.Fatalf("candidatos deveriam ter Context distintos, ambos são %q", candidates[0].Context)
	}
}

func TestSuggest_ValueNotPresent(t *testing.T) {
	text := "texto qualquer sem o número procurado"
	candidates := Suggest(text, "99999")
	if candidates == nil {
		t.Fatal("Suggest não deve devolver nil")
	}
	if len(candidates) != 0 {
		t.Fatalf("esperava slice vazio, obteve %d candidatos", len(candidates))
	}
}

// TestSuggest_EmptyValue garante que value vazio não causa loop infinito
// (o bug clássico de procurar strings.Index de "" repetidamente). Roda
// Suggest em uma goroutine e usa um timeout curto: se Suggest travar, o
// teste falha em vez de pendurar a suíte inteira.
func TestSuggest_EmptyValue(t *testing.T) {
	done := make(chan []Candidate, 1)
	go func() {
		done <- Suggest("qualquer texto aqui", "")
	}()

	select {
	case candidates := <-done:
		if candidates == nil {
			t.Fatal("Suggest não deve devolver nil para value vazio")
		}
		if len(candidates) != 0 {
			t.Fatalf("esperava slice vazio para value vazio, obteve %d", len(candidates))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Suggest entrou em loop infinito com value vazio")
	}
}

func TestSuggest_ValueAtStartOfLine(t *testing.T) {
	text := "12345 é o código do produto"
	value := "12345"

	candidates := Suggest(text, value)
	if len(candidates) != 1 {
		t.Fatalf("esperava 1 candidato, obteve %d: %+v", len(candidates), candidates)
	}

	c := candidates[0]
	if c.Index != 0 {
		t.Fatalf("esperava Index 0, obteve %d", c.Index)
	}

	captured, ok, err := Preview(c.Pattern, text)
	if err != nil {
		t.Fatalf("Preview retornou erro inesperado: %v", err)
	}
	if !ok {
		t.Fatalf("Preview não casou; pattern=%q", c.Pattern)
	}
	if captured != value {
		t.Fatalf("Preview capturou %q, esperava %q", captured, value)
	}
}

func TestSuggest_AccentedAnchor(t *testing.T) {
	text := "Documento fiscal\nEmissão nº: 999\nFim"
	value := "999"

	candidates := Suggest(text, value)
	if len(candidates) != 1 {
		t.Fatalf("esperava 1 candidato, obteve %d: %+v", len(candidates), candidates)
	}

	c := candidates[0]
	if !utf8.ValidString(c.Context) {
		t.Fatalf("Context não é UTF-8 válido: %q", c.Context)
	}

	captured, ok, err := Preview(c.Pattern, text)
	if err != nil {
		t.Fatalf("Preview retornou erro inesperado: %v", err)
	}
	if !ok {
		t.Fatalf("Preview não casou; pattern=%q", c.Pattern)
	}
	if captured != value {
		t.Fatalf("Preview capturou %q, esperava %q", captured, value)
	}
}

func TestSuggest_AnchorWithRegexMetacharacters(t *testing.T) {
	text := "Total (R$): 150"
	value := "150"

	candidates := Suggest(text, value)
	if len(candidates) != 1 {
		t.Fatalf("esperava 1 candidato, obteve %d: %+v", len(candidates), candidates)
	}

	c := candidates[0]
	captured, ok, err := Preview(c.Pattern, text)
	if err != nil {
		t.Fatalf("pattern %q não compilou / erro inesperado: %v", c.Pattern, err)
	}
	if !ok {
		t.Fatalf("Preview não casou; pattern=%q", c.Pattern)
	}
	if captured != value {
		t.Fatalf("Preview capturou %q, esperava %q", captured, value)
	}
}

func TestValueClass(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{"12345", `\d+`},
		{"ABC", `[A-Za-z]+`},
		{"AB12", `[A-Za-z0-9]+`},
	}

	for _, tt := range tests {
		got := ValueClass(tt.value)
		if got != tt.want {
			t.Errorf("ValueClass(%q) = %q, esperava %q", tt.value, got, tt.want)
		}
	}

	// Caso "outra coisa": deve ser o literal escapado, e não uma das
	// classes genéricas.
	literal := ValueClass("12-34")
	if literal == `\d+` || literal == `[A-Za-z]+` || literal == `[A-Za-z0-9]+` {
		t.Fatalf("ValueClass(%q) deveria ser literal escapado, obteve %q", "12-34", literal)
	}
	if literal != regexp.QuoteMeta("12-34") {
		t.Fatalf("ValueClass(%q) = %q, esperava regexp.QuoteMeta = %q", "12-34", literal, regexp.QuoteMeta("12-34"))
	}
	re, err := regexp.Compile(literal)
	if err != nil {
		t.Fatalf("literal de ValueClass não compila: %v", err)
	}
	if !re.MatchString("12-34") {
		t.Fatalf("literal de ValueClass %q não casa com o valor original", literal)
	}
}

func TestPreview_WithCaptureGroup(t *testing.T) {
	captured, ok, err := Preview(`NF:\s*(\d+)`, "NF: 42")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !ok {
		t.Fatal("esperava ok=true")
	}
	if captured != "42" {
		t.Fatalf("esperava captura '42', obteve %q", captured)
	}
}

func TestPreview_WithoutCaptureGroup(t *testing.T) {
	captured, ok, err := Preview(`\d+`, "NF: 42")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !ok {
		t.Fatal("esperava ok=true")
	}
	if captured != "42" {
		t.Fatalf("esperava match inteiro '42', obteve %q", captured)
	}
}

func TestPreview_NoMatch(t *testing.T) {
	_, ok, err := Preview(`\d+`, "sem números aqui")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if ok {
		t.Fatal("esperava ok=false")
	}
}

func TestPreview_InvalidPattern(t *testing.T) {
	_, ok, err := Preview("[", "qualquer texto")
	if err == nil {
		t.Fatal("esperava erro para regex inválida")
	}
	if ok {
		t.Fatal("esperava ok=false quando há erro")
	}
}

// TestSuggest_AllCandidatesRoundTrip é a garantia mais importante do
// pacote: para todo candidato devolvido por Suggest sobre um texto de
// exemplo, aplicar Preview de volta sobre o mesmo texto deve casar e
// capturar exatamente o valor original.
func TestSuggest_AllCandidatesRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		value string
	}{
		{
			name:  "nota fiscal simples",
			text:  "NOTA FISCAL\nNF: 12345\nEmitida em 01/01/2026",
			value: "12345",
		},
		{
			name:  "valor repetido",
			text:  "Pedido: 999\nTotal do pedido: 999 unidades",
			value: "999",
		},
		{
			name:  "valor no início do texto",
			text:  "12345 é o código do produto",
			value: "12345",
		},
		{
			name:  "âncora acentuada",
			text:  "Documento fiscal\nEmissão nº: 999\nFim",
			value: "999",
		},
		{
			name:  "âncora com metacaracteres",
			text:  "Total (R$): 150",
			value: "150",
		},
		{
			name:  "valor alfanumérico",
			text:  "Código do lote: AB12 - válido até 2027",
			value: "AB12",
		},
		{
			name:  "valor só letras",
			text:  "Status atual: ATIVO agora",
			value: "ATIVO",
		},
		{
			name:  "múltiplas linhas com tabs",
			text:  "Campo:\tXPTO\nOutro campo:\tXPTO extra",
			value: "XPTO",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidates := Suggest(tc.text, tc.value)
			if len(candidates) == 0 {
				t.Fatalf("Suggest não devolveu candidatos para o caso %q", tc.name)
			}
			for _, c := range candidates {
				if !utf8.ValidString(c.Context) {
					t.Errorf("Context inválido em UTF-8: %q (pattern=%q)", c.Context, c.Pattern)
				}
				captured, ok, err := Preview(c.Pattern, tc.text)
				if err != nil {
					t.Fatalf("pattern %q não compilou: %v", c.Pattern, err)
				}
				if !ok {
					t.Fatalf("pattern %q não casou com o texto de origem", c.Pattern)
				}
				if captured != tc.value {
					t.Fatalf("pattern %q capturou %q, esperava %q", c.Pattern, captured, tc.value)
				}
			}
		})
	}
}
