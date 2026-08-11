package organizepdf

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"

	"github.com/SamuelGFDias/file-manager/internal/ocr"
	"github.com/SamuelGFDias/file-manager/internal/pdfutil"
	"github.com/SamuelGFDias/file-manager/internal/ui"
	"github.com/SamuelGFDias/file-manager/internal/ui/calibrate"
	"github.com/SamuelGFDias/file-manager/internal/ui/filepicker"
)

// maxCalibrationCycles limita quantas vezes o usuário pode recalibrar um
// nível e testar de novo antes de aplicar, para proteger contra laço
// infinito.
const maxCalibrationCycles = 10

// maxSourceDirAttempts e maxSampleAttempts limitam, respectivamente, quantas
// vezes o usuário pode escolher de novo a pasta de origem (quando ela não
// tem PDF nenhum) e o arquivo de amostra (quando ele avisa que a amostra
// está fora da pasta de origem e recusa continuar assim mesmo), para
// proteger contra laço infinito.
const (
	maxSourceDirAttempts = 5
	maxSampleAttempts    = 5
)

// totalConfigSteps é a quantidade de etapas principais exibidas via
// ui.Step() durante o fluxo interativo de organize-pdf: pasta de origem,
// pasta de destino, modo de OCR, hierarquia de pastas (etapa 4 — pelo
// conteúdo do PDF, calibrando níveis, OU por uma planilha CSV; as duas
// variantes cabem na mesma etapa numerada, ver configureLevels), nome do
// arquivo, copiar/mover, relatório da execução e teste de calibragem.
// Puramente de apresentação — não afeta a lógica de configuração nem de
// processamento.
const totalConfigSteps = 8

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
	inputDir, pdfCount, err := t.pickInputDir()
	if err != nil {
		return "", err
	}
	t.opts.InputDir = inputDir
	ui.Blank()

	// Continua a navegação a partir da pasta de origem recém-selecionada, em
	// vez de reabrir em "." (diretório de trabalho do processo — na prática
	// a pasta onde o executável foi deixado). Na esmagadora maioria dos
	// casos o destino é a mesma pasta, uma irmã ou uma subpasta dela.
	ui.Step(2, totalConfigSteps, "Pasta de destino")
	outputDir, err := filepicker.PickDirWithPrompt(
		inputDir,
		"Selecione a PASTA DE DESTINO (onde a estrutura será criada)",
	)
	if err != nil {
		return "", err
	}
	t.opts.OutputDir = outputDir
	ui.Blank()

	if err := t.askOCRMode(); err != nil {
		return "", err
	}
	ui.Blank()

	samplePath, err := t.pickSample()
	if err != nil {
		return "", err
	}
	ui.Blank()

	// Crítico: a calibração precisa enxergar exatamente o mesmo texto que o
	// processamento vai enxergar depois. Usar t.textOptions() aqui garante
	// que os dois lados (calibração e runWith) nunca divirjam nas opções de
	// OCR — calibrar contra texto sem OCR e processar com OCR (ou o
	// contrário) faria a regex parecer certa ou errada por um motivo que
	// não tem nada a ver com a regex em si.
	textOpts, err := t.textOptions()
	if err != nil {
		return "", err
	}

	text, extractErr := pdfutil.ExtractTextOpts(context.Background(), samplePath, textOpts)
	if extractErr != nil {
		ui.Warnf("não foi possível extrair texto de %s: %v", ui.PathText(samplePath), extractErr)
		text = ""
	}

	if err := t.configureLevels(text); err != nil {
		return "", err
	}
	ui.Blank()

	if err := t.configureFilenameRegex(text); err != nil {
		return "", err
	}
	ui.Blank()

	if err := t.configureCopyOrMove(); err != nil {
		return "", err
	}
	ui.Blank()

	if err := t.configureReport(); err != nil {
		return "", err
	}
	ui.Blank()

	t.showConfigSummary(pdfCount)

	return text, nil
}

// pickInputDir pergunta a pasta de origem e barra a seleção enquanto ela não
// tiver nenhum PDF: sem isso o usuário percorre toda a calibração para só no
// fim descobrir, com "0 de 0 arquivos organizados", que escolheu a pasta
// errada — foi exatamente esse o bug relatado. Limitado a
// maxSourceDirAttempts tentativas para proteger contra laço infinito.
//
// O primeiro prompt começa em "." (é o primeiro, não há contexto anterior).
// Se a pasta escolhida não tiver PDF e o usuário optar por tentar de novo, o
// próximo prompt continua da pasta que ele acabou de tentar, não do zero —
// senão ele reprova exatamente o mesmo caminho de navegação de novo.
func (t *Tool) pickInputDir() (dir string, pdfCount int, err error) {
	ui.Step(1, totalConfigSteps, "Pasta de origem")

	start := "."
	for attempt := 0; attempt < maxSourceDirAttempts; attempt++ {
		dir, err = filepicker.PickDirWithPrompt(
			start,
			"Selecione a PASTA DE ORIGEM (onde estão os PDFs a organizar)",
		)
		if err != nil {
			return "", 0, err
		}
		start = dir // se for preciso tentar de novo, continua daqui

		count, countErr := countPDFs(dir)
		if countErr != nil {
			return "", 0, countErr
		}

		if count > 0 {
			ui.Infof("%s encontrados na pasta de origem.", ui.Count(count, "PDF", "PDFs"))
			return dir, count, nil
		}

		absDir, absErr := filepath.Abs(dir)
		if absErr != nil {
			absDir = dir
		}
		ui.Warnf("a pasta selecionada (%s) não contém nenhum arquivo PDF.", ui.PathText(absDir))

		choice := ""
		if err := survey.AskOne(&survey.Select{
			Message: "O que deseja fazer?",
			Options: []string{"Escolher outra pasta", "Cancelar"},
		}, &choice); err != nil {
			return "", 0, err
		}
		if choice == "Cancelar" {
			return "", 0, filepicker.ErrCancelled
		}
		// "Escolher outra pasta": tenta de novo.
	}

	return "", 0, filepicker.ErrCancelled
}

// pickSample pergunta o PDF de amostra e avisa quando ele está fora da pasta
// de origem: calibrar contra um documento que não faz parte do lote a
// processar é legítimo (o usuário pode ter um exemplar representativo
// guardado em outro lugar), mas fazer isso sem perceber foi a causa raiz do
// bug relatado — o seletor de amostra abre a partir da pasta de origem, mas
// nada impede de navegar para fora dela. Limitado a maxSampleAttempts
// tentativas para proteger contra laço infinito.
func (t *Tool) pickSample() (string, error) {
	for attempt := 0; attempt < maxSampleAttempts; attempt++ {
		samplePath, err := filepicker.PickFileWithPrompt(
			t.opts.InputDir,
			"Selecione um PDF de AMOSTRA (usado só para calibrar as regras)",
			[]string{".pdf"},
		)
		if err != nil {
			return "", err
		}

		outside, outsideErr := sampleOutsideInput(samplePath, t.opts.InputDir)
		if outsideErr != nil {
			return "", outsideErr
		}
		if !outside {
			return samplePath, nil
		}

		ui.Warnf(
			"o PDF de amostra escolhido (%s) não está dentro da pasta de origem (%s); "+
				"as regras serão calibradas contra um documento que não faz parte do lote a processar.",
			ui.PathText(samplePath), ui.PathText(t.opts.InputDir),
		)

		useAnyway := false
		if err := survey.AskOne(&survey.Confirm{
			Message: "Deseja continuar mesmo assim?",
			Default: false,
		}, &useAnyway); err != nil {
			return "", err
		}
		if useAnyway {
			return samplePath, nil
		}
		// Recusou: escolhe a amostra de novo.
	}

	return "", filepicker.ErrCancelled
}

// showConfigSummary mostra, antes do ciclo de teste de calibragem, um
// resumo com a pasta de origem, quantos PDFs foram encontrados nela, a
// pasta de destino e se a operação vai copiar ou mover. É a última chance
// do usuário perceber que selecionou a pasta errada antes de qualquer
// processamento.
func (t *Tool) showConfigSummary(pdfCount int) {
	inputAbs, err := filepath.Abs(t.opts.InputDir)
	if err != nil {
		inputAbs = t.opts.InputDir
	}
	outputAbs, err := filepath.Abs(t.opts.OutputDir)
	if err != nil {
		outputAbs = t.opts.OutputDir
	}

	action := "copiados"
	if t.opts.Move {
		action = "movidos"
	}

	ui.Divider()
	ui.Infof(
		"%s: %s em %s serão %s para %s.",
		ui.Bold("Resumo"), ui.Count(pdfCount, "PDF", "PDFs"), ui.PathText(inputAbs), action, ui.PathText(outputAbs),
	)
	ui.Divider()
	ui.Blank()
}

// countPDFs devolve quantos arquivos PDF há no diretório.
func countPDFs(dir string) (int, error) {
	entries, err := filepicker.ListDir(dir, []string{".pdf"})
	if err != nil {
		return 0, err
	}

	count := 0
	for _, e := range entries {
		if !e.IsDir {
			count++
		}
	}
	return count, nil
}

// sampleOutsideInput informa se o arquivo de amostra está fora da pasta de
// origem. Compara caminhos absolutos e limpos dos dois lados — comparar
// strings cruas falharia com "./pasta" versus "/home/x/pasta" apontando
// para o mesmo lugar.
func sampleOutsideInput(samplePath, inputDir string) (bool, error) {
	sampleAbs, err := filepath.Abs(samplePath)
	if err != nil {
		return false, fmt.Errorf("erro ao obter caminho absoluto de %s: %w", samplePath, err)
	}
	sampleDir := filepath.Clean(filepath.Dir(sampleAbs))

	inputAbs, err := filepath.Abs(inputDir)
	if err != nil {
		return false, fmt.Errorf("erro ao obter caminho absoluto de %s: %w", inputDir, err)
	}
	inputAbs = filepath.Clean(inputAbs)

	return sampleDir != inputAbs, nil
}

// csvHierarchyOption e contentHierarchyOption são as duas respostas
// possíveis à pergunta "Como definir as pastas de destino?" (ver
// configureLevels). Nomeadas como constantes porque comparadas em mais de
// um ponto (a resposta em si, e a opção default ao recalibrar).
const (
	contentHierarchyOption = "Pelo conteúdo de cada PDF (calibrar regras)"
	csvHierarchyOption     = "Por uma planilha CSV"
)

// configureLevels pergunta, primeiro, COMO a hierarquia de pastas de
// destino vai ser definida: pelo conteúdo de cada PDF (o fluxo original,
// calibrando um nível por vez — ver configureLevelsFromContent) ou por uma
// planilha que já diz onde cada documento deve ser arquivado, com o PDF
// fornecendo só a chave (ver configureCSVHierarchy). As duas opções
// resultam num destino mutuamente exclusivo: escolher uma limpa o que a
// outra teria configurado (t.opts.Levels vs. t.opts.CSV/CSVKeyRegex/...) —
// necessário porque Edit() reaproveita este mesmo fluxo para reeditar um
// perfil salvo, que pode ter sido configurado no outro modo da vez
// anterior.
func (t *Tool) configureLevels(sampleText string) error {
	ui.Step(4, totalConfigSteps, "Hierarquia de pastas")

	choice := ""
	if err := survey.AskOne(&survey.Select{
		Message: "Como definir as pastas de destino?",
		Options: []string{contentHierarchyOption, csvHierarchyOption},
	}, &choice); err != nil {
		return err
	}

	if choice == csvHierarchyOption {
		t.opts.Levels = nil
		return t.configureCSVHierarchy(sampleText)
	}

	t.opts.CSV = ""
	t.opts.CSVKeyRegex = ""
	t.opts.CSVKeyColumn = ""
	t.opts.CSVLevels = nil
	return t.configureLevelsFromContent(sampleText)
}

// configureLevelsFromContent laça perguntando se o usuário quer adicionar
// mais um nível de pasta, calibrando cada um por exemplo com
// calibrate.Calibrate. Responder "não" já na primeira pergunta é um
// caminho válido e esperado: é o modo "somente renomear", em que os
// arquivos vão direto para a pasta de destino, sem subpastas.
func (t *Tool) configureLevelsFromContent(sampleText string) error {
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

// maxCSVFileAttempts limita quantas vezes o usuário pode escolher de novo a
// planilha (quando a escolhida não pôde ser lida — coluna informada errada
// numa tentativa anterior, arquivo corrompido etc.), para proteger contra
// laço infinito, no mesmo espírito de maxSourceDirAttempts.
const maxCSVFileAttempts = 5

// configureCSVHierarchy conduz o fluxo interativo do modo --csv: escolher a
// planilha, mostrar um resumo (linhas, coluna-chave, colunas de hierarquia
// e um exemplo de caminho já normalizado) antes de qualquer processamento,
// oferecer a chance de trocar a coluna-chave ou escolher as colunas de
// hierarquia, e por fim calibrar a regex que extrai a chave do PDF —
// reaproveitando internal/ui/calibrate, o mesmo componente usado para
// calibrar níveis e nome de arquivo no modo por conteúdo.
func (t *Tool) configureCSVHierarchy(sampleText string) error {
	csvPath, err := filepicker.PickFileWithPrompt(
		t.opts.InputDir,
		"Selecione a PLANILHA que define a hierarquia de pastas de destino",
		[]string{".csv"},
	)
	if err != nil {
		return err
	}

	keyColumn := ""
	var levelColumns []string
	var loaded pdfutil.CSVMap

	for attempt := 0; attempt < maxCSVFileAttempts; attempt++ {
		m, loadErr := pdfutil.LoadCSVMap(csvPath, keyColumn, levelColumns)
		if loadErr == nil {
			loaded = m
			break
		}

		ui.Errorf("%v", loadErr)

		retry := true
		if err := survey.AskOne(&survey.Confirm{
			Message: "Escolher outra planilha?",
			Default: true,
		}, &retry); err != nil {
			return err
		}
		if !retry {
			return filepicker.ErrCancelled
		}

		newPath, err := filepicker.PickFileWithPrompt(
			t.opts.InputDir,
			"Selecione a PLANILHA que define a hierarquia de pastas de destino",
			[]string{".csv"},
		)
		if err != nil {
			return err
		}
		csvPath = newPath
		keyColumn = ""
		levelColumns = nil

		if attempt == maxCSVFileAttempts-1 {
			return fmt.Errorf("limite de %d tentativas de escolher uma planilha válida atingido", maxCSVFileAttempts)
		}
	}

	t.showCSVSummary(loaded)

	adjust := false
	if err := survey.AskOne(&survey.Confirm{
		Message: "Usar outra coluna como chave, ou escolher/reordenar as colunas de hierarquia?",
		Default: false,
	}, &adjust); err != nil {
		return err
	}

	if adjust {
		newKeyColumn, newLevelColumns, err := t.askCSVColumns(csvPath, loaded)
		if err != nil {
			return err
		}
		keyColumn = newKeyColumn
		levelColumns = newLevelColumns

		reloaded, err := pdfutil.LoadCSVMap(csvPath, keyColumn, levelColumns)
		if err != nil {
			return err
		}
		loaded = reloaded
		t.showCSVSummary(loaded)
	}

	pattern, err := calibrate.Calibrate(calibrate.Request{
		Label:      "chave do documento",
		SampleText: sampleText,
	})
	if err != nil {
		return err
	}

	t.opts.CSV = csvPath
	t.opts.CSVKeyRegex = pattern
	t.opts.CSVKeyColumn = keyColumn
	t.opts.CSVLevels = levelColumns

	return nil
}

// askCSVColumns lê o cabeçalho completo da planilha (todas as colunas,
// independente do que já estava selecionado) e deixa o usuário escolher
// qual vira a coluna-chave e quais (e em que ordem de exibição) formam a
// hierarquia de pastas.
func (t *Tool) askCSVColumns(csvPath string, current pdfutil.CSVMap) (keyColumn string, levelColumns []string, err error) {
	header, err := pdfutil.ReadCSVHeader(csvPath)
	if err != nil {
		return "", nil, err
	}

	keyColumn = current.KeyColumn
	if err := survey.AskOne(&survey.Select{
		Message: "Qual coluna é a chave (o valor que será procurado no PDF)?",
		Options: header,
		Default: current.KeyColumn,
	}, &keyColumn); err != nil {
		return "", nil, err
	}

	levelOptions := make([]string, 0, len(header)-1)
	for _, h := range header {
		if h == keyColumn {
			continue
		}
		levelOptions = append(levelOptions, h)
	}

	levelColumns = []string{}
	if err := survey.AskOne(&survey.MultiSelect{
		Message: "Quais colunas formam a hierarquia de pastas? (na ordem em que aparecem abaixo)",
		Options: levelOptions,
		Default: levelOptions,
	}, &levelColumns); err != nil {
		return "", nil, err
	}

	return keyColumn, levelColumns, nil
}

// showCSVSummary mostra, antes de calibrar a regex da chave (e antes de
// qualquer processamento de arquivo), quantas linhas a planilha tem, qual
// coluna foi tomada como chave, quais colunas viraram a hierarquia e um
// exemplo do caminho de destino que seria gerado a partir da primeira linha
// — ver o caminho pronto antes de qualquer processamento evita a
// descoberta tardia de que a coluna errada foi usada.
func (t *Tool) showCSVSummary(m pdfutil.CSVMap) {
	ui.Divider()
	ui.Infof("%s: %s na planilha.", ui.Bold("Resumo"), ui.Count(len(m.Rows), "linha", "linhas"))
	ui.Infof("Coluna-chave: %s", ui.Highlight(m.KeyColumn))
	ui.Infof("Colunas de hierarquia: %s", ui.Highlight(strings.Join(m.Levels, " / ")))

	if example, ok := exampleCSVPath(m); ok {
		ui.Infof("Exemplo de caminho gerado: %s", ui.PathText(example))
	}
	for _, w := range m.Warnings {
		ui.Warnf("%s", w)
	}
	ui.Divider()
	ui.Blank()
}

// exampleCSVPath monta o caminho de destino (componentes de pasta já
// normalizados + "chave.pdf") que a primeira linha lida da planilha geraria
// — usa m.Order (não m.Rows, que é um map sem ordem garantida) para achar
// de fato a primeira chave lida do arquivo. ok=false quando a planilha não
// tem nenhuma linha de dados (cabeçalho sozinho).
func exampleCSVPath(m pdfutil.CSVMap) (string, bool) {
	if len(m.Order) == 0 {
		return "", false
	}
	key := m.Order[0]
	components, ok := m.Lookup(key)
	if !ok {
		return "", false
	}
	parts := append([]string{}, components...)
	parts = append(parts, key+".pdf")
	return filepath.Join(parts...), true
}

// configureFilenameRegex pergunta se os arquivos devem ser renomeados a
// partir do conteúdo e, em caso afirmativo, calibra a regex de nome.
func (t *Tool) configureFilenameRegex(sampleText string) error {
	ui.Step(5, totalConfigSteps, "Nome do arquivo")

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

// askOCRMode pergunta, antes de qualquer calibração, se o OCR deve ser
// usado como recurso para PDFs sem texto embutido (digitalizados). Só faz
// a pergunta quando o Tesseract está de fato disponível no sistema: sem
// ele não há escolha real a fazer, então o modo é forçado para "never" e o
// usuário é avisado uma única vez — assim ele entende, já de saída, por
// que PDFs digitalizados vão cair em não-classificados.
func (t *Tool) askOCRMode() error {
	ui.Step(3, totalConfigSteps, "Modo de OCR")

	if !ocr.NewTesseract().Available() {
		t.opts.OCR = "never"
		ui.Infof(
			"OCR indisponível: Tesseract não encontrado. PDFs digitalizados (sem camada de texto) não serão lidos. %s",
			ocr.InstallHint(),
		)
		return nil
	}

	const (
		optAuto   = "Automático (recomendado)"
		optAlways = "Sempre"
		optNever  = "Nunca"
	)

	choice := ""
	if err := survey.AskOne(&survey.Select{
		Message: "Usar OCR em PDFs sem texto (digitalizados)?",
		Options: []string{optAuto, optAlways, optNever},
	}, &choice); err != nil {
		return err
	}

	switch choice {
	case optAlways:
		t.opts.OCR = "always"
	case optNever:
		t.opts.OCR = "never"
	default:
		t.opts.OCR = "auto"
	}

	return nil
}

// configureCopyOrMove pergunta se os arquivos devem ser copiados (padrão,
// não destrutivo) ou movidos.
func (t *Tool) configureCopyOrMove() error {
	ui.Step(6, totalConfigSteps, "Copiar ou mover")

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

// configureReport pergunta se deseja gerar um relatório desta execução
// (uma linha por arquivo considerado, classificado ou não, com o motivo) e,
// em caso afirmativo, o caminho onde gravá-lo. Default "sim": em contexto
// fiscal — o caso motivador desta ferramenta — poder conferir depois por
// que cada arquivo foi parar onde foi vale mais do que economizar uma
// pergunta. O formato (--report-format) não é perguntado aqui: fica no
// default "csv", ajustável via flag para quem for processar o relatório
// por programa.
func (t *Tool) configureReport() error {
	ui.Step(7, totalConfigSteps, "Relatório da execução")

	generate := true
	if err := survey.AskOne(&survey.Confirm{
		Message: "Gerar um relatório desta execução (arquivo, destino, classificado ou não, e o motivo)?",
		Default: true,
	}, &generate); err != nil {
		return err
	}

	if !generate {
		t.opts.Report = ""
		return nil
	}

	path := "./relatorio-organizacao.csv"
	if err := survey.AskOne(&survey.Input{
		Message: "Caminho do arquivo de relatório:",
		Default: path,
	}, &path); err != nil {
		return err
	}

	t.opts.Report = strings.TrimSpace(path)
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
		ui.Step(8, totalConfigSteps, "Teste de calibragem")

		sample, err := askTestSampleSize()
		if err != nil {
			return err
		}

		result, err := t.runWith(true, sample)
		if err != nil {
			return err
		}

		ui.Blank()
		ui.Successf("%s", result.Summary)
		for _, detail := range result.Details {
			ui.Warnf("%s", detail)
		}

		if t.ocrActive() {
			ui.Infof(
				"OCR foi usado para ler PDFs sem texto embutido; a leitura pode conter erros de reconhecimento " +
					"(caracteres trocados, palavras coladas etc.). Se algum arquivo esperado não casou, considere " +
					"afrouxar a regex antes de desistir.",
			)
		}
		ui.Blank()

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

// csvKeyOption é a opção de recalibrar a regex da chave do documento (modo
// --csv), oferecida por recalibrateLevel só quando t.opts.CSV não é vazio —
// nesse modo não há níveis calibrados por conteúdo (t.opts.Levels fica
// vazio, ver configureLevels), então a lista normal de níveis não teria o
// que oferecer sem essa opção extra.
const csvKeyOption = "Chave do documento (planilha)"

// recalibrateLevel mostra a lista de níveis já configurados (ou, em modo
// --csv, a opção de recalibrar a chave da planilha), mais a opção de
// recalibrar o nome do arquivo, deixa o usuário escolher qual quer refazer
// e chama calibrate.Calibrate com Initial preenchido com a regex atual
// daquele item, sem mexer nos demais.
func (t *Tool) recalibrateLevel(sampleText string) error {
	const filenameOption = "Nome do arquivo"

	options := make([]string, 0, len(t.opts.Levels)+2)
	if t.opts.CSV != "" {
		options = append(options, csvKeyOption)
	}
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

	if chosen == csvKeyOption {
		pattern, err := calibrate.Calibrate(calibrate.Request{
			Label:      "chave do documento",
			SampleText: sampleText,
			Initial:    t.opts.CSVKeyRegex,
		})
		if err != nil {
			return err
		}
		t.opts.CSVKeyRegex = pattern
		return nil
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
