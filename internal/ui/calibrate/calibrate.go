// Package calibrate implementa o diálogo interativo de "calibração de regex
// por exemplo": o usuário informa o valor que espera encontrar num PDF de
// amostra e o pacote descobre uma regex candidata, deixando o usuário
// confirmar ou ajustar antes de aceitar.
package calibrate

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"

	"github.com/SamuelGFDias/file-manager/internal/pdfutil"
	"github.com/SamuelGFDias/file-manager/internal/regexcalib"
	"github.com/SamuelGFDias/file-manager/internal/ui"
)

// ErrCancelled indica que o usuário abortou a calibração.
var ErrCancelled = errors.New("calibrate: calibração cancelada")

// errRetryValue é um sinal interno usado entre as etapas de revisão e de
// escolha de valor para indicar que o usuário quer voltar ao passo 2
// (digitar outro valor). Nunca escapa das funções exportadas do pacote.
var errRetryValue = errors.New("calibrate: tentar outro valor")

// maxRetries limita quantas vezes o usuário pode voltar a "tentar outro
// valor", para proteger contra laço infinito.
const maxRetries = 10

// maxContextRunes e maxPatternRunes limitam o tamanho, em runes, dos campos
// mostrados no rótulo de menu de um candidato.
const (
	maxContextRunes = 60
	maxPatternRunes = 40
)

// Request descreve uma calibração a ser feita.
type Request struct {
	Label      string // o que se está calibrando, ex "fornecedor" ou "número da nota"
	SampleText string // texto do PDF de amostra, já extraído
	Initial    string // regex atual, quando reeditando; vazio na primeira vez
}

// Calibrate conduz o diálogo e devolve a regex confirmada pelo usuário.
func Calibrate(req Request) (pattern string, err error) {
	// Reedição de um perfil já calibrado: pula direto para a revisão da
	// regex existente (passo 4), sem forçar o usuário a redigitar o valor.
	if req.Initial != "" {
		return reviewCycle(req, req.Initial)
	}

	if req.SampleText == "" {
		manual, err := askManualFallback()
		if err != nil {
			return "", err
		}
		if !manual {
			return "", ErrCancelled
		}
		return manualEntryLoop(req.Initial)
	}

	pattern, err = resolveValuePattern(req)
	if err != nil {
		return "", err
	}
	return reviewCycle(req, pattern)
}

// CalibrateFromFile extrai o texto do PDF e chama Calibrate.
func CalibrateFromFile(pdfPath, label, initial string) (pattern string, err error) {
	text, err := pdfutil.ExtractText(pdfPath)
	if err != nil {
		ui.Warnf("não foi possível extrair texto de %s: %v", pdfPath, err)
		text = ""
	}

	return Calibrate(Request{
		Label:      label,
		SampleText: text,
		Initial:    initial,
	})
}

// reviewCycle mostra a revisão (passo 4) de pattern; se o usuário pedir para
// tentar outro valor, volta ao passo 2/3 via resolveValuePattern e repete a
// revisão com o novo candidato. Limitado a maxRetries voltas.
func reviewCycle(req Request, pattern string) (string, error) {
	for attempt := 0; attempt < maxRetries; attempt++ {
		result, err := reviewLoop(req, pattern)
		if err == nil {
			return result, nil
		}
		if !errors.Is(err, errRetryValue) {
			return "", err
		}

		newPattern, err := resolveValuePattern(req)
		if err != nil {
			return "", err
		}
		pattern = newPattern
	}

	ui.Warnf("limite de tentativas atingido ao procurar o valor no documento.")
	return "", ErrCancelled
}

// resolveValuePattern conduz os passos 2 e 3: pergunta o valor esperado,
// sugere candidatos com regexcalib.Suggest e resolve para uma única regex.
// Limitado a maxRetries voltas de "tentar outro valor".
func resolveValuePattern(req Request) (string, error) {
	for attempt := 0; attempt < maxRetries; attempt++ {
		value, err := askValue(req.Label)
		if err != nil {
			return "", err
		}

		candidates := regexcalib.Suggest(req.SampleText, value)

		switch len(candidates) {
		case 0:
			action, err := askNoCandidateAction()
			if err != nil {
				return "", err
			}
			switch action {
			case actionRetryValue:
				continue
			case actionManualEntry:
				return manualEntryLoop("")
			default:
				return "", ErrCancelled
			}
		case 1:
			return candidates[0].Pattern, nil
		default:
			chosen, err := askCandidateChoice(candidates)
			if err != nil {
				return "", err
			}
			if chosen == "" {
				return "", ErrCancelled
			}
			return chosen, nil
		}
	}

	ui.Warnf("limite de tentativas atingido ao procurar o valor no documento.")
	return "", ErrCancelled
}

// askManualFallback avisa que o texto do PDF de amostra não pôde ser
// extraído e pergunta se o usuário quer digitar a regex manualmente.
func askManualFallback() (bool, error) {
	ui.Warnf("não foi possível extrair texto do PDF de amostra — provavelmente é um PDF digitalizado (imagem sem camada de texto) e esta ferramenta não faz OCR.")

	confirm := false
	err := survey.AskOne(&survey.Confirm{
		Message: "Deseja digitar a regex manualmente?",
	}, &confirm)
	if err != nil {
		if errors.Is(err, terminal.InterruptErr) {
			return false, ErrCancelled
		}
		return false, err
	}

	return confirm, nil
}

// manualEntryLoop pede ao usuário para digitar uma regex diretamente,
// pré-preenchida com def, validando até compilar (ou até o usuário
// cancelar).
func manualEntryLoop(def string) (string, error) {
	current := def
	for {
		newPattern, err := askManualPattern(current)
		if err != nil {
			return "", err
		}

		if _, compErr := regexp.Compile(newPattern); compErr != nil {
			ui.Errorf("regex inválida: %v", compErr)
			current = newPattern
			continue
		}

		return newPattern, nil
	}
}

// reviewLoop mostra a regex atual e o resultado do teste sobre o texto de
// amostra, e conduz as ações de confirmar, editar ou tentar outro valor
// (passo 4). Devolve errRetryValue se o usuário pedir para tentar outro
// valor.
func reviewLoop(req Request, pattern string) (string, error) {
	current := pattern
	for {
		description, _ := TestPattern(current, req.SampleText)
		ui.Infof("Regex: %s", current)
		ui.Infof("Resultado: %s", description)

		action := ""
		err := survey.AskOne(&survey.Select{
			Message: "O que deseja fazer?",
			Options: []string{
				"Confirmar",
				"Editar a regex manualmente",
				"Tentar outro valor",
				"Cancelar",
			},
		}, &action)
		if err != nil {
			if errors.Is(err, terminal.InterruptErr) {
				return "", ErrCancelled
			}
			return "", err
		}

		switch action {
		case "Confirmar":
			return current, nil
		case "Editar a regex manualmente":
			edited, err := manualEntryLoop(current)
			if err != nil {
				return "", err
			}
			current = edited
		case "Tentar outro valor":
			return "", errRetryValue
		default:
			return "", ErrCancelled
		}
	}
}

// askManualPattern pede ao usuário que digite uma regex, pré-preenchida com
// def.
func askManualPattern(def string) (string, error) {
	pattern := ""
	err := survey.AskOne(&survey.Input{
		Message: "Digite a regex (deve conter um grupo de captura):",
		Default: def,
	}, &pattern)
	if err != nil {
		if errors.Is(err, terminal.InterruptErr) {
			return "", ErrCancelled
		}
		return "", err
	}
	return pattern, nil
}

// askValue pergunta ao usuário o valor que deve ser encontrado no documento.
func askValue(label string) (string, error) {
	value := ""
	err := survey.AskOne(&survey.Input{
		Message: fmt.Sprintf("Qual o valor que deve ser encontrado no documento para %q?\n(copie exatamente como aparece no PDF)", label),
	}, &value)
	if err != nil {
		if errors.Is(err, terminal.InterruptErr) {
			return "", ErrCancelled
		}
		return "", err
	}
	return value, nil
}

// Ações possíveis quando nenhum candidato é encontrado (passo 3).
const (
	actionRetryValue = iota
	actionManualEntry
	actionCancel
)

// askNoCandidateAction pergunta o que fazer quando o valor não foi
// encontrado no texto do documento.
func askNoCandidateAction() (int, error) {
	ui.Warnf("o valor informado não foi encontrado no texto do documento.")

	options := []string{
		"Tentar outro valor",
		"Digitar a regex manualmente",
		"Cancelar",
	}

	selected := ""
	err := survey.AskOne(&survey.Select{
		Message: "O que deseja fazer?",
		Options: options,
	}, &selected)
	if err != nil {
		if errors.Is(err, terminal.InterruptErr) {
			return actionCancel, ErrCancelled
		}
		return actionCancel, err
	}

	switch selected {
	case "Tentar outro valor":
		return actionRetryValue, nil
	case "Digitar a regex manualmente":
		return actionManualEntry, nil
	default:
		return actionCancel, nil
	}
}

// askCandidateChoice mostra os candidatos encontrados para o usuário
// escolher qual ocorrência é a certa (passo 3, vários candidatos). Devolve
// "" se o usuário cancelar.
func askCandidateChoice(candidates []regexcalib.Candidate) (string, error) {
	options := make([]string, 0, len(candidates)+1)
	labelToPattern := make(map[string]string, len(candidates))

	for _, c := range candidates {
		label := FormatCandidate(c)
		options = append(options, label)
		labelToPattern[label] = c.Pattern
	}
	options = append(options, "Cancelar")

	selected := ""
	err := survey.AskOne(&survey.Select{
		Message: "Vários trechos casam com esse valor. Qual ocorrência é a certa?",
		Options: options,
	}, &selected)
	if err != nil {
		if errors.Is(err, terminal.InterruptErr) {
			return "", ErrCancelled
		}
		return "", err
	}

	if selected == "Cancelar" {
		return "", nil
	}

	return labelToPattern[selected], nil
}

// FormatCandidate monta o rótulo de menu de um candidato (função pura,
// testável).
func FormatCandidate(c regexcalib.Candidate) string {
	context := truncateRunes(c.Context, maxContextRunes)
	pattern := truncateRunes(c.Pattern, maxPatternRunes)
	return fmt.Sprintf("%s   →   %s", context, pattern)
}

// truncateRunes corta s em no máximo max runes, acrescentando "…" quando
// truncar. A contagem e o corte são feitos por rune, não por byte, para
// nunca quebrar um caractere UTF-8 multibyte ao meio.
func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

// TestPattern aplica a regex sobre o texto e descreve o resultado em
// português (função pura, testável). ok indica se houve match.
func TestPattern(pattern, text string) (description string, ok bool) {
	captured, matched, err := regexcalib.Preview(pattern, text)
	if err != nil {
		return fmt.Sprintf("regex inválida: %v", err), false
	}
	if !matched {
		return "nenhum trecho do documento casa com esta regex", false
	}
	return fmt.Sprintf("capturou: %q", captured), true
}
