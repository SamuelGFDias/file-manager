package docs

import (
	"errors"
	"path/filepath"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"

	"github.com/SamuelGFDias/file-manager/internal/tool"
	"github.com/SamuelGFDias/file-manager/internal/ui"
)

const (
	optionExportContext = "Exportar documentação de contexto (para colar numa IA)"
	optionExportSkill   = "Exportar skill (para instalar num agente de IA)"
	optionBack          = "Voltar"

	defaultContextPath = "./file-manager-docs.md"
	defaultSkillPath   = "./SKILL.md"
)

// docsScreen é a tela interativa de exportação de documentação.
type docsScreen struct {
	tools   []tool.Tool
	version string
}

// NewScreen devolve a tela interativa de documentação, que permite ao
// usuário exportar a documentação de contexto ou a skill de agente de IA
// para um arquivo.
func NewScreen(tools []tool.Tool, version string) ui.Screen {
	return &docsScreen{tools: tools, version: version}
}

// Title devolve o título da tela, usado no breadcrumb de navegação.
func (s *docsScreen) Title() string {
	return "Documentação"
}

// Run executa a tela: pergunta qual formato exportar (ou volta), pergunta o
// caminho de saída e grava o arquivo.
func (s *docsScreen) Run(nav *ui.Navigator) error {
	choice := ""
	err := survey.AskOne(&survey.Select{
		Message: "O que você deseja fazer?",
		Options: []string{optionExportContext, optionExportSkill, optionBack},
	}, &choice)
	if err != nil {
		if errors.Is(err, terminal.InterruptErr) {
			nav.Pop()
			return nil
		}
		return err
	}

	var format Format
	var defaultPath string

	switch choice {
	case optionExportContext:
		format = FormatContext
		defaultPath = defaultContextPath
	case optionExportSkill:
		format = FormatSkill
		defaultPath = defaultSkillPath
	default:
		nav.Pop()
		return nil
	}

	path := defaultPath
	err = survey.AskOne(&survey.Input{
		Message: "Caminho de saída",
		Default: defaultPath,
	}, &path)
	if err != nil {
		if errors.Is(err, terminal.InterruptErr) {
			nav.Pop()
			return nil
		}
		return err
	}

	if err := Export(format, path, s.tools, s.version); err != nil {
		ui.Errorf("erro ao exportar documentação: %v", err)
		ui.Pause()
		return nil
	}

	abs, absErr := filepath.Abs(path)
	if absErr != nil {
		abs = path
	}

	ui.Successf("Documentação exportada em %s", abs)
	ui.Pause()

	return nil
}
