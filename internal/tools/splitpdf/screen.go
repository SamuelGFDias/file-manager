package splitpdf

import (
	"errors"

	"github.com/SamuelGFDias/file-manager/internal/tool"
	"github.com/SamuelGFDias/file-manager/internal/ui"
	"github.com/SamuelGFDias/file-manager/internal/ui/filepicker"
)

// screen é a tela interativa da ferramenta split-pdf.
type screen struct {
	tool *Tool
}

// Title devolve o título exibido no breadcrumb.
func (s *screen) Title() string {
	return "Separar PDFs"
}

// Run reseta as opções para os defaults, faz as perguntas interativas,
// executa a separação e mostra o resultado antes de voltar à tela
// anterior.
func (s *screen) Run(nav *ui.Navigator) error {
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
