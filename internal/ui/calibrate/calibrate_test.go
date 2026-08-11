package calibrate

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/SamuelGFDias/file-manager/internal/regexcalib"
)

func TestFormatCandidate_ShortContext(t *testing.T) {
	c := regexcalib.Candidate{
		Context: "nota fiscal 12345 emitida",
		Pattern: `nota fiscal (\d+)`,
	}

	got := FormatCandidate(c)

	if strings.Contains(got, "…") {
		t.Errorf("contexto curto não deveria ser truncado, got: %q", got)
	}
	if !strings.Contains(got, c.Context) {
		t.Errorf("esperava conter o contexto inteiro %q, got: %q", c.Context, got)
	}
	if !strings.Contains(got, c.Pattern) {
		t.Errorf("esperava conter o pattern inteiro %q, got: %q", c.Pattern, got)
	}
}

func TestFormatCandidate_LongContextTruncated(t *testing.T) {
	longContext := strings.Repeat("a", 100)
	c := regexcalib.Candidate{
		Context: longContext,
		Pattern: `(\d+)`,
	}

	got := FormatCandidate(c)

	if !strings.Contains(got, "…") {
		t.Errorf("contexto longo deveria ser truncado com reticências, got: %q", got)
	}

	// A parte de contexto do rótulo não deve ultrapassar maxContextRunes+1
	// (o "…" extra) runes.
	parts := strings.SplitN(got, "   →   ", 2)
	if len(parts) != 2 {
		t.Fatalf("esperava separador '   →   ' no rótulo, got: %q", got)
	}
	contextPart := parts[0]
	runeCount := utf8.RuneCountInString(contextPart)
	if runeCount > maxContextRunes+1 {
		t.Errorf("contexto truncado tem %d runes, esperava no máximo %d", runeCount, maxContextRunes+1)
	}
}

func TestFormatCandidate_AccentsAndEmojiNearCutoff_ValidUTF8(t *testing.T) {
	// Monta um contexto com acentuação e emoji posicionados perto do ponto
	// de corte (rune 60), para garantir que o truncamento é feito por rune
	// e não por byte (o que quebraria caracteres multibyte ao meio).
	prefix := strings.Repeat("ção ", 14) // 4 runes cada, ~56 runes
	context := prefix + "🎉 número da nota é 12345 e mais texto depois disso para garantir corte"
	c := regexcalib.Candidate{
		Context: context,
		Pattern: `(\d+)`,
	}

	got := FormatCandidate(c)

	if !utf8.ValidString(got) {
		t.Fatalf("resultado não é UTF-8 válido: %q", got)
	}
}

func TestFormatCandidate_LongPatternTruncated(t *testing.T) {
	longPattern := `nota fiscal n[uú]mero\s*(\d+)` + strings.Repeat("x", 60)
	c := regexcalib.Candidate{
		Context: "curto",
		Pattern: longPattern,
	}

	got := FormatCandidate(c)

	parts := strings.SplitN(got, "   →   ", 2)
	if len(parts) != 2 {
		t.Fatalf("esperava separador '   →   ' no rótulo, got: %q", got)
	}
	patternPart := parts[1]

	if !strings.Contains(patternPart, "…") {
		t.Errorf("pattern longo deveria ser truncado com reticências, got: %q", patternPart)
	}

	runeCount := utf8.RuneCountInString(patternPart)
	if runeCount > maxPatternRunes+1 {
		t.Errorf("pattern truncado tem %d runes, esperava no máximo %d", runeCount, maxPatternRunes+1)
	}
}

func TestTestPattern_MatchWithCapture(t *testing.T) {
	pattern := `nota fiscal (\d+)`
	text := "a nota fiscal 12345 foi emitida ontem"

	description, ok := TestPattern(pattern, text)

	if !ok {
		t.Fatalf("esperava ok=true, got false (description: %q)", description)
	}
	if !strings.Contains(description, "12345") {
		t.Errorf("esperava que a descrição contivesse o valor capturado 12345, got: %q", description)
	}
}

func TestTestPattern_NoMatch(t *testing.T) {
	pattern := `nota fiscal (\d+)`
	text := "este documento não tem o que procuramos"

	description, ok := TestPattern(pattern, text)

	if ok {
		t.Fatalf("esperava ok=false, got true (description: %q)", description)
	}
	if !strings.Contains(description, "nenhum trecho") {
		t.Errorf("esperava descrição de 'nenhum trecho', got: %q", description)
	}
}

func TestTestPattern_InvalidRegex(t *testing.T) {
	pattern := "["
	text := "qualquer texto"

	description, ok := TestPattern(pattern, text)

	if ok {
		t.Fatalf("esperava ok=false para regex inválida, got true (description: %q)", description)
	}
	if !strings.HasPrefix(description, "regex inválida") {
		t.Errorf("esperava descrição começando com 'regex inválida', got: %q", description)
	}
}
