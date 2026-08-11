// Package mainmenu implementa a tela inicial do modo interativo do CLI
// file-manager: o menu que lista as ferramentas registradas e dá acesso às
// telas de perfis e de documentação.
package mainmenu

import (
	"errors"
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"

	"github.com/SamuelGFDias/file-manager/internal/selfupdate"
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

	updateChecker *selfupdate.Checker
}

// NewScreen devolve a tela do menu principal, listando as ferramentas
// informadas (mais "Perfis", quando alguma delas suportar perfis salvos,
// "Documentação" e "Sair").
//
// É também o ponto de entrada de toda sessão interativa, então é aqui que
// ui.ApplyTheme() é chamada — uma única vez, de forma idempotente — para
// que o template de seleção do survey (descrição só na opção selecionada,
// dica em português) já esteja em vigor antes do primeiro prompt.
//
// version é a versão formatada exibida no cabeçalho do menu (ex: "0.1.0
// (abc1234, 2026-08-11T12:00:00Z)"); currentVersion é a versão semântica
// crua (ex: "v0.1.0", ou "dev" em builds locais), usada para checar em
// segundo plano se há uma versão mais nova publicada. A checagem é
// disparada uma única vez aqui (Start é idempotente e não bloqueia) para
// que, se houver aviso, ele já esteja pronto quando o rodapé do menu for
// desenhado, sem nunca atrasar a abertura do menu nem repetir a consulta a
// cada redesenho da tela.
func NewScreen(tools []tool.Tool, version string, currentVersion string) ui.Screen {
	ui.ApplyTheme()

	checker := selfupdate.NewChecker(selfupdate.DefaultRepo, currentVersion)
	checker.Start()

	return &screen{tools: tools, version: version, updateChecker: checker}
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

	// Aviso de atualização, no rodapé do menu: impresso antes de abrir o
	// survey.Select porque, uma vez aberto, o select toma conta do terminal
	// e qualquer impressão feita depois some por trás dele. Notice() nunca
	// bloqueia: se a checagem em segundo plano ainda não terminou (ou falhou
	// silenciosamente, ou a versão local não é semver), simplesmente não há
	// aviso a mostrar.
	if notice, ok := s.updateChecker.Notice(); ok {
		ui.Warnf("%s", notice)
	}

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
