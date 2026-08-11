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

// multiSelectQuestionTemplatePT é uma cópia do template padrão de
// survey.MultiSelectQuestionTemplate (survey v2.3.7, multiselect.go), com um
// único ajuste: a dica em inglês
// "[Use arrows to move, space to select, <right> to all, <left> to none,
// type to filter]" foi traduzida para português, deixando explícito que a
// MARCAÇÃO é feita com a barra de espaço — Enter só confirma o que já
// estiver marcado.
//
// Essa dica é exatamente a informação cuja ausência em inglês, numa
// interface inteiramente em português, causou o defeito relatado: o usuário
// navegou até uma pasta com PDF, apertou Enter sem antes apertar espaço, e a
// seleção vazia resultante só foi percebida seis perguntas depois. A
// tradução do template de survey.Select (selectQuestionTemplatePT, acima)
// cobriu a escolha única na v0.8.0, mas o de escolha múltipla — usado por
// filepicker.PickFiles para selecionar vários arquivos de uma vez — ficou
// de fora e permaneceu em inglês até esta correção.
//
// Qualquer outra linha é mantida idêntica ao template original do survey,
// de propósito: o objetivo é traduzir o texto, não mudar o comportamento do
// prompt.
const multiSelectQuestionTemplatePT = `
{{- define "option"}}
    {{- if eq .SelectedIndex .CurrentIndex }}{{color .Config.Icons.SelectFocus.Format }}{{ .Config.Icons.SelectFocus.Text }}{{color "reset"}}{{else}} {{end}}
    {{- if index .Checked .CurrentOpt.Index }}{{color .Config.Icons.MarkedOption.Format }} {{ .Config.Icons.MarkedOption.Text }} {{else}}{{color .Config.Icons.UnmarkedOption.Format }} {{ .Config.Icons.UnmarkedOption.Text }} {{end}}
    {{- color "reset"}}
    {{- " "}}{{- .CurrentOpt.Value}}{{ if ne ($.GetDescription .CurrentOpt) "" }} - {{color "cyan"}}{{ $.GetDescription .CurrentOpt }}{{color "reset"}}{{end}}
{{end}}
{{- if .ShowHelp }}{{- color .Config.Icons.Help.Format }}{{ .Config.Icons.Help.Text }} {{ .Help }}{{color "reset"}}{{"\n"}}{{end}}
{{- color .Config.Icons.Question.Format }}{{ .Config.Icons.Question.Text }} {{color "reset"}}
{{- color "default+hb"}}{{ .Message }}{{ .FilterMessage }}{{color "reset"}}
{{- if .ShowAnswer}}{{color "cyan"}} {{.Answer}}{{color "reset"}}{{"\n"}}
{{- else }}
	{{- "  "}}{{- color "cyan"}}[use ↑ ↓ para navegar, ESPAÇO para marcar,{{- if not .Config.RemoveSelectAll }} → marca todos,{{end}}{{- if not .Config.RemoveSelectNone }} ← desmarca todos,{{end}} digite para filtrar, Enter para confirmar{{- if and .Help (not .ShowHelp)}}, {{ .Config.HelpInput }} para mais ajuda{{end}}]{{color "reset"}}
  {{- "\n"}}
  {{- range $ix, $option := .PageEntries}}
    {{- template "option" $.IterateOption $ix $option}}
  {{- end}}
{{- end}}`

var applyThemeOnce sync.Once

// ApplyTheme sobrescreve survey.SelectQuestionTemplate e
// survey.MultiSelectQuestionTemplate: a descrição de uma opção de
// survey.Select só aparece quando essa opção estiver selecionada (em vez de
// todas ao mesmo tempo), e as dicas de navegação de ambos os templates são
// traduzidas para português — a de MultiSelect destacando que a marcação é
// feita com a barra de espaço. É idempotente — chamadas repetidas não têm
// custo extra nem efeito colateral além da primeira.
//
// Os demais templates do survey usados neste CLI (Confirm, Input) não
// precisam de tradução: o único texto em inglês que carregam fica atrás de
// `{{if .Help}}` / `{{if .Suggest}}`, e nenhuma pergunta deste programa
// define Help ou Suggest — esse texto nunca chega a ser exibido. Password
// não é usado em lugar nenhum do CLI.
func ApplyTheme() {
	applyThemeOnce.Do(func() {
		survey.SelectQuestionTemplate = selectQuestionTemplatePT
		survey.MultiSelectQuestionTemplate = multiSelectQuestionTemplatePT
	})
}
