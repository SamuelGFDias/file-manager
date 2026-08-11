package organizepdf

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"

	"github.com/SamuelGFDias/file-manager/internal/pdfutil"
	"github.com/SamuelGFDias/file-manager/internal/ui"
	"github.com/SamuelGFDias/file-manager/internal/ui/calibrate"
	"github.com/SamuelGFDias/file-manager/internal/ui/filepicker"
)

// maxCalibrationCycles limita quantas vezes o usuário pode recalibrar um
// nível e testar de novo antes de aplicar, para proteger contra laço
// infinito.
const maxCalibrationCycles = 10

// screen é a tela interativa da ferramenta organize-pdf.
type screen struct {
	tool *Tool
}

// Title devolve o título exibido no breadcrumb.
func (s *screen) Title() string {
	return "Organizar PDFs"
}

// Run conduz o fluxo interativo completo de organize-pdf: escolha das
// pastas, calibração dos níveis de pasta e do nome do arquivo, um teste
// obrigatório da calibração (com possibilidade de recalibrar) antes de
// qualquer alteração real, e só então a aplicação de fato.
func (s *screen) Run(nav *ui.Navigator) error {
	s.tool.opts = defaultOptions()
	s.tool.rawLevels = nil

	if err := s.tool.askProfileOrConfigureNow(); err != nil {
		return s.finish(nav, err)
	}

	sampleText, err := s.tool.configure()
	if err != nil {
		return s.finish(nav, err)
	}

	if err := s.testAndApplyCycle(nav, sampleText); err != nil {
		return s.finish(nav, err)
	}

	return nil
}

// finish trata o erro final de qualquer etapa do fluxo interativo:
// cancelamentos explícitos do usuário (calibrate.ErrCancelled ou
// filepicker.ErrCancelled) apenas voltam à tela anterior em silêncio;
// qualquer outro erro é mostrado antes de voltar. Sempre devolve nil — o
// próprio Run() já tratou o problema, não há nada a propagar ao
// Navigator.
func (s *screen) finish(nav *ui.Navigator, err error) error {
	if err == nil {
		nav.Pop()
		return nil
	}
	if errors.Is(err, calibrate.ErrCancelled) || errors.Is(err, filepicker.ErrCancelled) {
		nav.Pop()
		return nil
	}
	ui.Errorf("%v", err)
	ui.Pause()
	nav.Pop()
	return nil
}

// askProfileOrConfigureNow pergunta se o usuário quer usar um perfil salvo
// ou configurar agora. A escolha de perfis salvos não é reimplementada
// aqui: é feita pela tela genérica "Perfis" do menu principal. Ao escolher
// essa opção aqui, apenas avisamos e seguimos para a configuração normal.
func (t *Tool) askProfileOrConfigureNow() error {
	choice := ""
	if err := survey.AskOne(&survey.Select{
		Message: "Como deseja configurar a organização?",
		Options: []string{
			"Configurar agora",
			"Usar um perfil salvo",
		},
	}, &choice); err != nil {
		return err
	}

	if choice == "Usar um perfil salvo" {
		ui.Infof("Perfis salvos são geridos pela tela \"Perfis\" do menu principal. Vamos configurar agora.")
	}

	return nil
}

// configure conduz as perguntas de configuração de organize-pdf: pasta de
// origem e destino, escolha de um PDF de amostra (cujo texto é extraído
// UMA única vez e reaproveitado em todas as calibrações a seguir), o laço
// de níveis de pasta, a calibração opcional do nome do arquivo e a escolha
// entre copiar e mover. Devolve o texto extraído do PDF de amostra, para
// que quem chamou possa reusá-lo (ex: numa recalibração posterior sem
// pedir um novo arquivo de amostra).
//
// É reusada tanto pela tela interativa (screen.Run, antes do ciclo de
// teste) quanto por Profile().Edit, ao reeditar um perfil salvo.
func (t *Tool) configure() (sampleText string, err error) {
	inputDir, err := filepicker.PickDir(".")
	if err != nil {
		return "", err
	}
	t.opts.InputDir = inputDir

	outputDir, err := filepicker.PickDir(".")
	if err != nil {
		return "", err
	}
	t.opts.OutputDir = outputDir

	samplePath, err := filepicker.PickFile(t.opts.InputDir, []string{".pdf"})
	if err != nil {
		return "", err
	}

	text, extractErr := pdfutil.ExtractText(samplePath)
	if extractErr != nil {
		ui.Warnf("não foi possível extrair texto de %s: %v", samplePath, extractErr)
		text = ""
	}

	if err := t.configureLevels(text); err != nil {
		return "", err
	}

	if err := t.configureFilenameRegex(text); err != nil {
		return "", err
	}

	if err := t.configureCopyOrMove(); err != nil {
		return "", err
	}

	return text, nil
}

// configureLevels laça perguntando se o usuário quer adicionar mais um
// nível de pasta, calibrando cada um por exemplo com calibrate.Calibrate.
// Responder "não" já na primeira pergunta é um caminho válido e esperado:
// é o modo "somente renomear", em que os arquivos vão direto para a pasta
// de destino, sem subpastas.
func (t *Tool) configureLevels(sampleText string) error {
	t.opts.Levels = nil

	for {
		add := false
		message := "Adicionar um nível de pasta? (responda \"não\" para apenas renomear os arquivos, sem criar subpastas)"
		if err := survey.AskOne(&survey.Confirm{
			Message: message,
			Default: len(t.opts.Levels) == 0,
		}, &add); err != nil {
			return err
		}
		if !add {
			return nil
		}

		label := ""
		if err := survey.AskOne(&survey.Input{
			Message: "Rótulo deste nível (ex: fornecedor):",
		}, &label); err != nil {
			return err
		}
		label = strings.TrimSpace(label)
		if label == "" {
			ui.Warnf("rótulo vazio, tente novamente.")
			continue
		}

		pattern, err := calibrate.Calibrate(calibrate.Request{
			Label:      label,
			SampleText: sampleText,
		})
		if err != nil {
			return err
		}

		t.opts.Levels = append(t.opts.Levels, LevelSpec{Label: label, Regex: pattern})
	}
}

// configureFilenameRegex pergunta se os arquivos devem ser renomeados a
// partir do conteúdo e, em caso afirmativo, calibra a regex de nome.
func (t *Tool) configureFilenameRegex(sampleText string) error {
	rename := true
	if err := survey.AskOne(&survey.Confirm{
		Message: "Renomear os arquivos com base no conteúdo?",
		Default: true,
	}, &rename); err != nil {
		return err
	}

	if !rename {
		t.opts.FilenameRegex = ""
		return nil
	}

	pattern, err := calibrate.Calibrate(calibrate.Request{
		Label:      "nome do arquivo",
		SampleText: sampleText,
	})
	if err != nil {
		return err
	}

	t.opts.FilenameRegex = pattern
	return nil
}

// configureCopyOrMove pergunta se os arquivos devem ser copiados (padrão,
// não destrutivo) ou movidos.
func (t *Tool) configureCopyOrMove() error {
	choice := ""
	if err := survey.AskOne(&survey.Select{
		Message: "Copiar ou mover os arquivos?",
		Options: []string{
			"Copiar (mantém os originais)",
			"Mover",
		},
	}, &choice); err != nil {
		return err
	}

	t.opts.Move = choice == "Mover"
	return nil
}

// testAndApplyCycle conduz o coração da usabilidade de organize-pdf: antes
// de qualquer alteração real, testa a calibração atual em modo simulação
// (contra todos os arquivos ou uma amostra), mostra o resultado e deixa o
// usuário aplicar, recalibrar um nível específico e testar de novo, ou
// cancelar. Limitado a maxCalibrationCycles voltas para proteger contra
// laço infinito.
func (s *screen) testAndApplyCycle(nav *ui.Navigator, sampleText string) error {
	t := s.tool

	for cycle := 0; cycle < maxCalibrationCycles; cycle++ {
		sample, err := askTestSampleSize()
		if err != nil {
			return err
		}

		result, err := t.runWith(true, sample)
		if err != nil {
			return err
		}

		ui.Successf("%s", result.Summary)
		for _, detail := range result.Details {
			ui.Warnf("%s", detail)
		}

		action := ""
		if err := survey.AskOne(&survey.Select{
			Message: "O que deseja fazer?",
			Options: []string{
				"Aplicar agora",
				"Recalibrar um nível",
				"Cancelar",
			},
		}, &action); err != nil {
			return err
		}

		switch action {
		case "Aplicar agora":
			applyResult, err := t.runWith(false, 0)
			if err != nil {
				return err
			}
			ui.Successf("%s", applyResult.Summary)
			for _, detail := range applyResult.Details {
				ui.Infof("%s", detail)
			}
			ui.Pause()
			nav.Pop()
			return nil

		case "Recalibrar um nível":
			if err := t.recalibrateLevel(sampleText); err != nil {
				return err
			}
			continue

		default: // Cancelar
			nav.Pop()
			return nil
		}
	}

	ui.Warnf("limite de %d tentativas de calibragem atingido; cancelando.", maxCalibrationCycles)
	nav.Pop()
	return nil
}

// askTestSampleSize pergunta se o teste de calibração deve rodar contra
// todos os arquivos da pasta ou só uma amostra, pedindo a quantidade nesse
// segundo caso. Devolve 0 para "todos".
func askTestSampleSize() (int, error) {
	choice := ""
	if err := survey.AskOne(&survey.Select{
		Message: "Como deseja testar a calibragem, antes de aplicar de verdade?",
		Options: []string{
			"Testar com todos os arquivos da pasta (recomendado)",
			"Testar com uma amostra (informar quantidade)",
		},
	}, &choice); err != nil {
		return 0, err
	}

	if choice == "Testar com todos os arquivos da pasta (recomendado)" {
		return 0, nil
	}

	raw := ""
	if err := survey.AskOne(&survey.Input{
		Message: "Quantos arquivos testar?",
	}, &raw); err != nil {
		return 0, err
	}

	n, convErr := strconv.Atoi(strings.TrimSpace(raw))
	if convErr != nil {
		return 0, fmt.Errorf("quantidade de amostra inválida %q: %w", raw, convErr)
	}

	return n, nil
}

// recalibrateLevel mostra a lista de níveis já configurados, mais a opção
// de recalibrar o nome do arquivo, deixa o usuário escolher qual quer
// refazer e chama calibrate.Calibrate com Initial preenchido com a regex
// atual daquele nível, sem mexer nos demais.
func (t *Tool) recalibrateLevel(sampleText string) error {
	const filenameOption = "Nome do arquivo"

	options := make([]string, 0, len(t.opts.Levels)+1)
	for _, level := range t.opts.Levels {
		options = append(options, level.Label)
	}
	options = append(options, filenameOption)

	chosen := ""
	if err := survey.AskOne(&survey.Select{
		Message: "Qual nível deseja recalibrar?",
		Options: options,
	}, &chosen); err != nil {
		return err
	}

	if chosen == filenameOption {
		pattern, err := calibrate.Calibrate(calibrate.Request{
			Label:      "nome do arquivo",
			SampleText: sampleText,
			Initial:    t.opts.FilenameRegex,
		})
		if err != nil {
			return err
		}
		t.opts.FilenameRegex = pattern
		return nil
	}

	for i, level := range t.opts.Levels {
		if level.Label != chosen {
			continue
		}
		pattern, err := calibrate.Calibrate(calibrate.Request{
			Label:      level.Label,
			SampleText: sampleText,
			Initial:    level.Regex,
		})
		if err != nil {
			return err
		}
		t.opts.Levels[i].Regex = pattern
		return nil
	}

	return nil
}
