package mergepdf

import (
	"errors"

	"github.com/SamuelGFDias/file-manager/internal/tool"
	"github.com/SamuelGFDias/file-manager/internal/ui"
	"github.com/SamuelGFDias/file-manager/internal/ui/filepicker"
)

// Screen é a tela interativa de merge-pdf.
type Screen struct {
	tool *Tool
}

// Title devolve o título exibido no breadcrumb.
func (s *Screen) Title() string {
	return "Unir PDFs"
}

// Run pergunta os parâmetros de merge-pdf, executa a ferramenta e mostra o
// resultado antes de voltar para a tela anterior.
func (s *Screen) Run(nav *ui.Navigator) error {
	s.tool.opts = defaultOptions()

	if err := tool.PromptAll(s.tool.params()); err != nil {
		if errors.Is(err, filepicker.ErrCancelled) {
			nav.Pop()
			return nil
		}
		ui.Errorf("%v", err)
		ui.Pause()
		nav.Pop()
		return nil
	}

	result, err := s.tool.run()
	if err != nil {
		ui.Errorf("%v", err)
		ui.Pause()
		nav.Pop()
		return nil
	}

	ui.Successf("%s", result.Summary)
	for _, d := range result.Details {
		ui.Infof("%s", d)
	}

	ui.Pause()
	nav.Pop()
	return nil
}
