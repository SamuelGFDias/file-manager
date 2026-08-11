// Package mainmenu implementa a tela inicial do modo interativo do CLI
// file-manager: o menu que lista as ferramentas registradas e dá acesso às
// telas de perfis e de documentação.
package mainmenu

import (
	"errors"
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"

	"github.com/SamuelGFDias/file-manager/internal/tool"
	"github.com/SamuelGFDias/file-manager/internal/ui"
	"github.com/SamuelGFDias/file-manager/internal/ui/docs"
	"github.com/SamuelGFDias/file-manager/internal/ui/profiles"
)

const (
	optionProfiles = "Perfis"
	optionDocs     = "Documentação"
	optionExit     = "Sair"
)

// screen é a implementação de ui.Screen para o menu principal.
type screen struct {
	tools   []tool.Tool
	version string
}

// NewScreen devolve a tela do menu principal, listando as ferramentas
// informadas (mais "Perfis", quando alguma delas suportar perfis salvos,
// "Documentação" e "Sair").
func NewScreen(tools []tool.Tool, version string) ui.Screen {
	return &screen{tools: tools, version: version}
}

// Title devolve o título da tela, usado no breadcrumb de navegação.
func (s *screen) Title() string {
	return "File Manager"
}

// Run mostra o menu principal e navega para a tela correspondente à opção
// escolhida.
func (s *screen) Run(nav *ui.Navigator) error {
	options := make([]string, 0, len(s.tools)+3)
	descriptions := make(map[string]string, len(s.tools))
	toolByLabel := make(map[string]tool.Tool, len(s.tools))

	for _, t := range s.tools {
		meta := t.Meta()
		options = append(options, meta.Title)
		descriptions[meta.Title] = meta.Description
		toolByLabel[meta.Title] = t
	}

	if len(profiles.SupportingTools(s.tools)) > 0 {
		options = append(options, optionProfiles)
	}
	options = append(options, optionDocs, optionExit)

	choice := ""
	err := survey.AskOne(&survey.Select{
		Message: fmt.Sprintf("File Manager (%s) — o que você deseja fazer?", s.version),
		Options: options,
		Description: func(value string, index int) string {
			return descriptions[value]
		},
	}, &choice)
	if err != nil {
		if errors.Is(err, terminal.InterruptErr) {
			nav.Exit()
			return nil
		}
		return err
	}

	switch choice {
	case optionProfiles:
		nav.Push(profiles.NewScreen(s.tools))
	case optionDocs:
		nav.Push(docs.NewScreen(s.tools, s.version))
	case optionExit:
		nav.Exit()
	default:
		if t, ok := toolByLabel[choice]; ok {
			nav.Push(t.Screen())
		}
	}

	return nil
}
