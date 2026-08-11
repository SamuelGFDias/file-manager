package ocrpdf

import (
	"errors"

	"github.com/AlecAivazis/survey/v2"
	"github.com/SamuelGFDias/file-manager/internal/tool"
	"github.com/SamuelGFDias/file-manager/internal/ui"
	"github.com/SamuelGFDias/file-manager/internal/ui/filepicker"
)

// totalConfigSteps é a quantidade de etapas principais exibidas via
// ui.Step() no fluxo interativo: configuração, simulação e aplicação.
// Puramente de apresentação.
const totalConfigSteps = 3

// Screen é a tela interativa de ocr-pdf.
type Screen struct {
	tool *Tool
}

// Title devolve o título exibido no breadcrumb.
func (s *Screen) Title() string {
	return "Tornar PDF pesquisável"
}

// uiProgress é o Progress usado pela tela interativa: mesma linha de
// formatProgressLine (comando cobra), impressa com ui.Infof em vez de
// fmt.Println, para seguir a mesma aparência do resto da interface.
func uiProgress(done, total int, path string) {
	ui.Infof("%s", formatProgressLine(done, total, path))
}

// Run conduz o fluxo interativo de ocr-pdf: pergunta a configuração, roda
// SEMPRE uma simulação primeiro (mostrando quantos arquivos são elegíveis
// e quantos seriam pulados, e por quê) e só aplica de verdade depois de
// confirmação explícita — mesmo espírito do ciclo de teste antes de
// aplicar já usado por organize-pdf: nunca tocar em arquivo nenhum sem que
// o usuário tenha visto o que vai acontecer.
func (s *Screen) Run(nav *ui.Navigator) error {
	s.tool.opts = defaultOptions()

	if err := tool.PromptAll(s.tool.params()); err != nil {
		return s.finish(nav, err)
	}

	ui.Blank()
	ui.Step(2, totalConfigSteps, "Simulando (nenhum arquivo será alterado ainda)")
	ui.Blank()

	dryRaw, dryResult, err := s.tool.ocrizeRaw(true, uiProgress)
	if err != nil {
		return s.finish(nav, err)
	}

	ui.Successf("%s", dryResult.Summary)
	for _, d := range dryResult.Details {
		ui.Infof("%s", d)
	}
	ui.Blank()

	if len(dryRaw.Processed) == 0 {
		ui.Warnf("Nenhum arquivo elegível encontrado — nada para aplicar.")
		ui.Pause()
		nav.Pop()
		return nil
	}

	var proceed bool
	if err := survey.AskOne(&survey.Confirm{
		Message: "Aplicar de verdade com esta configuração?",
		Default: false,
	}, &proceed); err != nil {
		return s.finish(nav, err)
	}
	if !proceed {
		ui.Infof("Operação cancelada. Nada foi alterado.")
		ui.Pause()
		nav.Pop()
		return nil
	}

	ui.Blank()
	ui.Step(3, totalConfigSteps, "Aplicando")
	ui.Blank()

	result, err := s.tool.runWith(false, uiProgress)
	if err != nil {
		return s.finish(nav, err)
	}

	ui.Successf("%s", result.Summary)
	for _, d := range result.Details {
		ui.Infof("%s", d)
	}

	ui.Pause()
	nav.Pop()
	return nil
}

// finish trata o erro final de qualquer etapa do fluxo interativo:
// cancelamentos explícitos do usuário (filepicker.ErrCancelled) apenas
// voltam à tela anterior em silêncio; qualquer outro erro é mostrado antes
// de voltar. Sempre devolve nil.
func (s *Screen) finish(nav *ui.Navigator, err error) error {
	if errors.Is(err, filepicker.ErrCancelled) {
		nav.Pop()
		return nil
	}
	ui.Errorf("%v", err)
	ui.Pause()
	nav.Pop()
	return nil
}
