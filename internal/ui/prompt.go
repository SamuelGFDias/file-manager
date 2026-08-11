package ui

import (
	"sync"

	"github.com/AlecAivazis/survey/v2"
)

// selectQuestionTemplatePT é uma cópia do template padrão de
// survey.SelectQuestionTemplate (survey v2.3.7, select.go), com dois ajustes:
//
//  1. A descrição de uma opção só é exibida quando ela é a opção atualmente
//     selecionada (acompanha a seta), em vez de aparecer para todas as
//     opções ao mesmo tempo — era a principal reclamação de poluição visual
//     na tela.
//  2. A dica em inglês "[Use arrows to move, type to filter]" foi traduzida
//     para português, já que era o único texto em inglês na interface.
//
// Qualquer outra linha é mantida idêntica ao template original do survey,
// de propósito: o objetivo é uma mudança cirúrgica de apresentação, não uma
// reescrita do comportamento do prompt.
const selectQuestionTemplatePT = `
{{- define "option"}}
    {{- if eq .SelectedIndex .CurrentIndex }}{{color .Config.Icons.SelectFocus.Format }}{{ .Config.Icons.SelectFocus.Text }} {{else}}{{color "default"}}  {{end}}
    {{- .CurrentOpt.Value}}{{ if and (eq .SelectedIndex .CurrentIndex) (ne ($.GetDescription .CurrentOpt) "") }} - {{color "cyan"}}{{ $.GetDescription .CurrentOpt }}{{end}}
    {{- color "reset"}}
{{end}}
{{- if .ShowHelp }}{{- color .Config.Icons.Help.Format }}{{ .Config.Icons.Help.Text }} {{ .Help }}{{color "reset"}}{{"\n"}}{{end}}
{{- color .Config.Icons.Question.Format }}{{ .Config.Icons.Question.Text }} {{color "reset"}}
{{- color "default+hb"}}{{ .Message }}{{ .FilterMessage }}{{color "reset"}}
{{- if .ShowAnswer}}{{color "cyan"}} {{.Answer}}{{color "reset"}}{{"\n"}}
{{- else}}
  {{- "  "}}{{- color "cyan"}}[use ↑ ↓ para navegar, digite para filtrar, Enter para confirmar{{- if and .Help (not .ShowHelp)}}, {{ .Config.HelpInput }} para mais ajuda{{end}}]{{color "reset"}}
  {{- "\n"}}
  {{- range $ix, $option := .PageEntries}}
    {{- template "option" $.IterateOption $ix $option}}
  {{- end}}
{{- end}}`

var applyThemeOnce sync.Once

// ApplyTheme sobrescreve survey.SelectQuestionTemplate para que a descrição
// de uma opção de survey.Select só apareça quando essa opção estiver
// selecionada (em vez de todas ao mesmo tempo), e traduz a dica de navegação
// para português. É idempotente — chamadas repetidas não têm custo extra
// nem efeito colateral além da primeira.
func ApplyTheme() {
	applyThemeOnce.Do(func() {
		survey.SelectQuestionTemplate = selectQuestionTemplatePT
	})
}
