// Package regexcalib implementa a "calibração por exemplo": a partir de um
// valor conhecido (ex.: o número de uma nota fiscal) e do texto onde ele foi
// encontrado, sugere expressões regulares candidatas que capturam esse
// mesmo valor em documentos semelhantes.
//
// O pacote é lógica pura, sem I/O: recebe texto e valor em memória e devolve
// dados (candidatos, previews). A camada de CLI é responsável por extrair o
// texto do PDF e apresentar as sugestões ao usuário.
package regexcalib

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// maxAnchorBytes é o tamanho máximo (em bytes) da âncora à esquerda usada
// para identificar o campo antes do valor.
const maxAnchorBytes = 40

// contextRadius é o número de bytes de contexto mostrados antes e depois da
// ocorrência do valor, para o usuário decidir qual candidato faz sentido.
const contextRadius = 30

// Candidate é uma sugestão de regex para capturar o valor procurado.
type Candidate struct {
	Pattern string // a regex sugerida, com exatamente um grupo de captura
	Context string // trecho do texto ao redor da ocorrência, para o usuário escolher qual serve
	Index   int    // posição (byte offset) da ocorrência no texto original
}

// Suggest encontra todas as ocorrências literais de value em text e devolve
// uma regex candidata para cada uma. Devolve slice vazio (nunca nil) se
// value não ocorre em text, ou se value for vazio.
func Suggest(text, value string) []Candidate {
	candidates := []Candidate{}

	// Valor vazio "ocorreria" em toda posição do texto (strings.Index de
	// string vazia sempre acha índice 0), o que levaria a um laço
	// infinito se avançássemos por len(value)==0. Corta o caso fora.
	if value == "" {
		return candidates
	}

	offset := 0
	for {
		rel := strings.Index(text[offset:], value)
		if rel == -1 {
			break
		}
		idx := offset + rel

		anchor := leftAnchor(text, idx)
		pattern := buildPattern(anchor, value)

		if _, err := regexp.Compile(pattern); err == nil {
			candidates = append(candidates, Candidate{
				Pattern: pattern,
				Context: buildContext(text, idx, len(value)),
				Index:   idx,
			})
		}

		offset = idx + len(value)
	}

	return candidates
}

// leftAnchor devolve o trecho de texto imediatamente antes de idx, indo
// para trás até (o que vier primeiro) uma quebra de linha ou
// maxAnchorBytes bytes. O início do trecho é ajustado para uma fronteira de
// rune válida.
func leftAnchor(text string, idx int) string {
	start := idx - maxAnchorBytes
	if start < 0 {
		start = 0
	}

	search := text[start:idx]
	if nlPos := strings.LastIndex(search, "\n"); nlPos != -1 {
		start = start + nlPos + 1
	}

	start = alignRuneStart(text, start)

	return text[start:idx]
}

// buildPattern monta a regex a partir da âncora (texto que precede o valor)
// e do valor em si.
func buildPattern(anchor, value string) string {
	class := ValueClass(value)

	anchorClean := strings.TrimRight(anchor, " \t")
	if anchorClean == "" {
		return "(" + class + ")"
	}

	hadTrailingSpace := len(anchorClean) != len(anchor)

	quoted := regexp.QuoteMeta(anchorClean)
	if hadTrailingSpace {
		return quoted + `\s*(` + class + ")"
	}
	return quoted + "(" + class + ")"
}

// buildContext monta o trecho de contexto ao redor da ocorrência, com até
// contextRadius bytes antes e depois, quebras de linha convertidas em
// espaço, espaços múltiplos colapsados e reticências indicando corte.
func buildContext(text string, idx, valueLen int) string {
	endIdx := idx + valueLen

	left := idx - contextRadius
	truncatedLeft := left > 0
	if left < 0 {
		left = 0
	}
	left = alignRuneStart(text, left)

	right := endIdx + contextRadius
	truncatedRight := right < len(text)
	if right > len(text) {
		right = len(text)
	}
	right = alignRuneStart(text, right)

	raw := text[left:right]
	raw = strings.ReplaceAll(raw, "\n", " ")
	raw = strings.ReplaceAll(raw, "\r", " ")
	raw = strings.Join(strings.Fields(raw), " ")

	if truncatedLeft {
		raw = "…" + raw
	}
	if truncatedRight {
		raw = raw + "…"
	}

	return raw
}

// alignRuneStart avança pos até a próxima fronteira de rune válida em s
// (ou até len(s)), garantindo que fatiar s a partir de pos nunca corte um
// caractere UTF-8 multibyte ao meio.
func alignRuneStart(s string, pos int) int {
	for pos < len(s) && !utf8.RuneStart(s[pos]) {
		pos++
	}
	return pos
}

// Preview aplica pattern sobre text e devolve o que seria capturado.
// ok=false se a regex não casar. Erro só se pattern for inválida.
func Preview(pattern, text string) (captured string, ok bool, err error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", false, err
	}

	match := re.FindStringSubmatch(text)
	if match == nil {
		return "", false, nil
	}

	if len(match) > 1 {
		return match[1], true, nil
	}
	return match[0], true, nil
}

// ValueClass devolve a classe de caractere generalizada para um valor:
// só dígitos -> `\d+`; só letras -> `[A-Za-z]+`; alfanumérico -> `[A-Za-z0-9]+`;
// qualquer outra coisa -> regexp.QuoteMeta(value) (literal escapado).
//
// A classificação usa somente o intervalo ASCII, para garantir que a classe
// devolvida efetivamente reconheça o valor original: um valor com letras
// acentuadas (ex.: "ção") não é "só letras" no sentido ASCII de [A-Za-z], e
// cai no caso literal.
func ValueClass(value string) string {
	if value == "" {
		return regexp.QuoteMeta(value)
	}

	allDigits := true
	allLetters := true
	for _, r := range value {
		if !isASCIIDigit(r) {
			allDigits = false
		}
		if !isASCIILetter(r) {
			allLetters = false
		}
	}

	switch {
	case allDigits:
		return `\d+`
	case allLetters:
		return `[A-Za-z]+`
	}

	allAlnum := true
	for _, r := range value {
		if !isASCIIDigit(r) && !isASCIILetter(r) {
			allAlnum = false
			break
		}
	}
	if allAlnum {
		return `[A-Za-z0-9]+`
	}

	return regexp.QuoteMeta(value)
}

func isASCIIDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func isASCIILetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}
