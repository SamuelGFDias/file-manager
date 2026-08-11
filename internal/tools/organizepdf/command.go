package organizepdf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/SamuelGFDias/file-manager/internal/history"
	"github.com/SamuelGFDias/file-manager/internal/ocr"
	"github.com/SamuelGFDias/file-manager/internal/pdfutil"
	"github.com/SamuelGFDias/file-manager/internal/tool"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// maxUnclassifiedDetails limita quantas linhas de arquivos não classificados
// aparecem em tool.Result.Details — despejar centenas de linhas numa tela de
// terminal não ajuda ninguém.
const maxUnclassifiedDetails = 10

// params declara, em um único lugar, todos os parâmetros aceitos pela
// ferramenta organize-pdf. Cada tool.Param liga o mesmo campo de t.opts (ou
// t.rawLevels, para --level) a uma flag do cobra (BindFlag) e à
// documentação gerada (via tool.DocFlags). Nenhum parâmetro tem Prompt: o
// modo interativo de organize-pdf segue um fluxo próprio, com calibração
// por exemplo e teste antes de aplicar (ver screen.go), que não se encaixa
// no modelo de "uma pergunta por parâmetro" usado pelas ferramentas mais
// simples.
func (t *Tool) params() []tool.Param {
	return []tool.Param{
		{
			Name:        "input",
			Shorthand:   "i",
			Type:        "string",
			Description: "Pasta com os PDFs a organizar",
			Default:     "",
			Example:     "./notas-fiscais",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVarP(&t.opts.InputDir, "input", "i", t.opts.InputDir, "Pasta com os PDFs a organizar")
			},
		},
		{
			Name:        "output",
			Shorthand:   "o",
			Type:        "string",
			Description: "Pasta de destino",
			Default:     "",
			Example:     "./organizado",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVarP(&t.opts.OutputDir, "output", "o", t.opts.OutputDir, "Pasta de destino")
			},
		},
		{
			Name:        "level",
			Shorthand:   "",
			Type:        "stringSlice",
			Description: `Nível de pasta no formato "rótulo=regex". Pode repetir; a ordem define a hierarquia`,
			Default:     "",
			Example:     `--level "fornecedor=FORNECEDOR:\s*(\w+)" --level "filial=FILIAL\s*(\d+)"`,
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringSliceVar(&t.rawLevels, "level", t.rawLevels, `Nível de pasta no formato "rótulo=regex". Pode repetir; a ordem define a hierarquia`)
			},
		},
		{
			Name:        "filename-regex",
			Shorthand:   "",
			Type:        "string",
			Description: "Regex cujo grupo de captura vira o nome do arquivo. Vazio = mantém o nome original",
			Default:     "",
			Example:     `NF:\s*(\d+)`,
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVar(&t.opts.FilenameRegex, "filename-regex", t.opts.FilenameRegex, "Regex cujo grupo de captura vira o nome do arquivo. Vazio = mantém o nome original")
			},
		},
		{
			Name:        "move",
			Shorthand:   "",
			Type:        "bool",
			Description: "Move em vez de copiar (o padrão é copiar, preservando a origem)",
			Default:     "false",
			Example:     "--move",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.BoolVar(&t.opts.Move, "move", t.opts.Move, "Move em vez de copiar (o padrão é copiar, preservando a origem)")
			},
		},
		{
			Name:        "unclassified-dir",
			Shorthand:   "",
			Type:        "string",
			Description: "Subpasta para os arquivos que não casaram",
			Default:     "sem-classificacao",
			Example:     "sem-classificacao",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVar(&t.opts.UnclassifiedDir, "unclassified-dir", t.opts.UnclassifiedDir, "Subpasta para os arquivos que não casaram")
			},
		},
		{
			Name:        "overwrite",
			Shorthand:   "",
			Type:        "bool",
			Description: "Sobrescreve arquivos já existentes no destino",
			Default:     "false",
			Example:     "--overwrite",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.BoolVar(&t.opts.Overwrite, "overwrite", t.opts.Overwrite, "Sobrescreve arquivos já existentes no destino")
			},
		},
		{
			Name:        "dry-run",
			Shorthand:   "",
			Type:        "bool",
			Description: "Só simula: mostra o destino calculado de cada arquivo sem copiar ou mover nada",
			Default:     "false",
			Example:     "--dry-run",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.BoolVar(&t.opts.DryRun, "dry-run", t.opts.DryRun, "Só simula: mostra o destino calculado de cada arquivo sem copiar ou mover nada")
			},
		},
		{
			Name:        "sample",
			Shorthand:   "",
			Type:        "int",
			Description: "Limita a simulação aos N primeiros arquivos (0 = todos)",
			Default:     "0",
			Example:     "--sample 5",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.IntVar(&t.opts.Sample, "sample", t.opts.Sample, "Limita a simulação aos N primeiros arquivos (0 = todos)")
			},
		},
		{
			Name:        "ocr",
			Shorthand:   "",
			Type:        "string",
			Description: `Uso de OCR em PDFs sem texto: "auto" (só quando não há texto), "always" ou "never"`,
			Default:     "auto",
			Example:     "--ocr always",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVar(&t.opts.OCR, "ocr", t.opts.OCR, `Uso de OCR em PDFs sem texto: "auto" (só quando não há texto), "always" ou "never"`)
			},
		},
		{
			Name:        "ocr-lang",
			Shorthand:   "",
			Type:        "string",
			Description: `Idioma do OCR (ex: "por", "eng")`,
			Default:     "por",
			Example:     "--ocr-lang eng",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVar(&t.opts.OCRLang, "ocr-lang", t.opts.OCRLang, `Idioma do OCR (ex: "por", "eng")`)
			},
		},
		{
			Name:      "report",
			Shorthand: "",
			Type:      "string",
			Description: "Caminho do arquivo de relatório desta execução (uma linha por arquivo considerado, " +
				"classificado ou não, com o motivo quando não classificado); vazio = não gera. Também funciona " +
				"com --dry-run — é aliás quando mais serve, para conferir a classificação antes de aplicar",
			Default: "",
			Example: "--report ./relatorio-organizacao.csv",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVar(&t.opts.Report, "report", t.opts.Report, "Caminho do arquivo de relatório desta execução; vazio = não gera")
			},
		},
		{
			Name:        "report-format",
			Shorthand:   "",
			Type:        "string",
			Description: `Formato do relatório gerado por --report: "csv" ou "json"`,
			Default:     "csv",
			Example:     "--report-format json",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVar(&t.opts.ReportFormat, "report-format", t.opts.ReportFormat, `Formato do relatório gerado por --report: "csv" ou "json"`)
			},
		},
		{
			Name:        "csv",
			Shorthand:   "",
			Type:        "string",
			Description: "Planilha que define a hierarquia de pastas de destino; vazio = hierarquia vem do conteúdo do PDF (--level). Incompatível com --level",
			Default:     "",
			Example:     `--csv ./planilha.csv --csv-key-regex "NOTA:\s*(\d+)"`,
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVar(&t.opts.CSV, "csv", t.opts.CSV, "Planilha que define a hierarquia de pastas de destino; vazio = hierarquia vem do conteúdo do PDF (--level)")
			},
		},
		{
			Name:        "csv-key-regex",
			Shorthand:   "",
			Type:        "string",
			Description: "Regex que extrai do PDF a chave usada para procurar a linha correspondente em --csv. Obrigatório junto com --csv",
			Default:     "",
			Example:     `NOTA:\s*(\d+)`,
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVar(&t.opts.CSVKeyRegex, "csv-key-regex", t.opts.CSVKeyRegex, "Regex que extrai do PDF a chave usada para procurar a linha correspondente em --csv")
			},
		},
		{
			Name:        "csv-key-column",
			Shorthand:   "",
			Type:        "string",
			Description: "Coluna da planilha (--csv) com a chave; vazio = primeira coluna do cabeçalho",
			Default:     "",
			Example:     "NOTA",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVar(&t.opts.CSVKeyColumn, "csv-key-column", t.opts.CSVKeyColumn, "Coluna da planilha (--csv) com a chave; vazio = primeira coluna do cabeçalho")
			},
		},
		{
			Name:        "csv-levels",
			Shorthand:   "",
			Type:        "stringSlice",
			Description: "Colunas da planilha (--csv) que formam a hierarquia de pastas, na ordem; vazio = todas menos a chave, na ordem do arquivo",
			Default:     "",
			Example:     "--csv-levels CIDADE --csv-levels BAIRRO",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringSliceVar(&t.opts.CSVLevels, "csv-levels", t.opts.CSVLevels, "Colunas da planilha (--csv) que formam a hierarquia de pastas, na ordem; vazio = todas menos a chave, na ordem do arquivo")
			},
		},
	}
}

// textOptions monta o pdfutil.TextOptions correspondente às opções atuais
// de OCR (t.opts.OCR / t.opts.OCRLang). É usado tanto por runWith quanto
// pela tela interativa (na calibração), para que os dois lados nunca
// possam divergir na extração de texto: calibrar contra um texto e
// processar com outro (ou vice-versa) produziria resultados sem sentido.
func (t *Tool) textOptions() (pdfutil.TextOptions, error) {
	mode, err := pdfutil.ParseOCRMode(t.opts.OCR)
	if err != nil {
		return pdfutil.TextOptions{}, err
	}

	lang := strings.TrimSpace(t.opts.OCRLang)
	if lang == "" {
		lang = "por"
	}

	opts := pdfutil.TextOptions{Mode: mode, Lang: lang}
	if mode != pdfutil.OCRNever {
		opts.Engine = ocr.NewTesseract()
	}

	return opts, nil
}

// ocrWarnings devolve avisos em português a acrescentar em Result.Details
// quando o OCR está configurado para ser usado (auto/always) mas o motor
// não está disponível, ou está disponível mas sem o pacote do idioma
// pedido. Não são erros: a execução prossegue normalmente, só sem OCR (ou
// com OCR de precisão reduzida) — silenciar isso faria o usuário ver
// arquivos "não classificados" sem entender a causa.
func ocrWarnings(opts pdfutil.TextOptions) []string {
	if opts.Mode == pdfutil.OCRNever {
		return nil
	}

	engine, ok := opts.Engine.(*ocr.Tesseract)
	if !ok || engine == nil {
		return nil
	}

	if !engine.Available() {
		return []string{fmt.Sprintf(
			"aviso: Tesseract não encontrado — PDFs digitalizados (sem texto embutido) não serão lidos por OCR. %s",
			ocr.InstallHint(),
		)}
	}

	if !engine.HasLanguage(opts.Lang) {
		return []string{fmt.Sprintf(
			"aviso: pacote de idioma %q do Tesseract não está instalado — o OCR vai cair no idioma padrão e a precisão do reconhecimento tende a ser baixa",
			opts.Lang,
		)}
	}

	return nil
}

// ocrActive reporta se o OCR está de fato em condições de ser usado com a
// configuração atual: modo diferente de "never" E motor disponível no
// sistema. Usado pela tela interativa para decidir se vale avisar o
// usuário, no resultado do teste de calibração, que a leitura passou por
// OCR (e portanto pode conter erros de reconhecimento).
func (t *Tool) ocrActive() bool {
	opts, err := t.textOptions()
	if err != nil || opts.Mode == pdfutil.OCRNever {
		return false
	}

	engine, ok := opts.Engine.(*ocr.Tesseract)
	return ok && engine != nil && engine.Available()
}

// Command devolve o subcomando cobra "organize-pdf".
func (t *Tool) Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "organize-pdf",
		Short: "Organiza PDFs em pastas de destino a partir do conteúdo",
		Long: "Classifica os PDFs de uma pasta numa hierarquia de pastas de destino, com base em valores " +
			"encontrados no texto de cada arquivo, e opcionalmente renomeia cada um pelo valor capturado. " +
			"A hierarquia de pastas segue a ordem em que --level é repetido; sem nenhum --level, a " +
			"ferramenta apenas renomeia os arquivos em lote, sem criar subpastas.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := t.run()
			if err != nil {
				return err
			}

			fmt.Println(result.Summary)
			for _, d := range result.Details {
				fmt.Println(d)
			}

			return nil
		},
	}

	tool.BindAll(cmd.Flags(), t.params())
	_ = cmd.MarkFlagRequired("input")
	_ = cmd.MarkFlagRequired("output")

	return cmd
}

// historyRecorder monta a função injetada em pdfutil.OrganizeOptions.
// Recorder: converte as RecordedEntry de uma execução real em
// history.Entry e grava um history.Manifest com history.Save. Fica aqui —
// no comando, não em pdfutil — de propósito: é o único ponto do CLI que
// conhece tanto pdfutil (o domínio de organização) quanto internal/history
// (o domínio de histórico/desfazer); nenhum dos dois pacotes de domínio
// precisa conhecer o outro. inputDir e outputDir devem ser caminhos
// absolutos: o comando "undo" pode rodar depois, de um diretório de
// trabalho diferente do usado nesta organização, então o manifesto precisa
// fazer sentido independente do cwd de quem o gravou.
func historyRecorder(inputDir, outputDir string) func(action string, entries []pdfutil.RecordedEntry) error {
	return func(action string, entries []pdfutil.RecordedEntry) error {
		histEntries := make([]history.Entry, 0, len(entries))
		for _, e := range entries {
			histEntries = append(histEntries, history.Entry{
				Source: e.Source,
				Dest:   e.Dest,
				Size:   e.Size,
			})
		}

		m := history.Manifest{
			Tool:      "organize-pdf",
			CreatedAt: time.Now(),
			InputDir:  inputDir,
			OutputDir: outputDir,
			Action:    history.Action(action),
			Entries:   histEntries,
		}

		_, err := history.Save(m)
		return err
	}
}

// writeReportFile monta as linhas do relatório a partir de result e grava
// no caminho path, no formato informado ("csv" ou "json" — já validado por
// NormalizeReportFormat antes de chegar aqui), criando os diretórios
// intermediários que faltarem. Devolve o caminho absoluto gravado, para a
// confirmação em tool.Result.Details.
//
// Erros aqui (ex: path aponta para um diretório sem permissão de escrita,
// ou para um diretório já existente) são devolvidos ao chamador, que NUNCA
// os trata como falha da organização em si (ver comentário em runWith) —
// a mesma regra já aplicada à falha de gravação do manifesto de histórico.
func writeReportFile(path, format string, result pdfutil.OrganizeResult) (string, error) {
	absPath, absErr := filepath.Abs(path)
	if absErr != nil {
		absPath = path
	}

	if dir := filepath.Dir(absPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("criar diretório do relatório %q: %w", dir, err)
		}
	}

	f, err := os.Create(absPath)
	if err != nil {
		return "", fmt.Errorf("criar arquivo de relatório %q: %w", absPath, err)
	}

	rows := pdfutil.BuildReport(result)

	var writeErr error
	if format == "json" {
		writeErr = pdfutil.WriteReportJSON(f, rows)
	} else {
		writeErr = pdfutil.WriteReportCSV(f, rows)
	}

	closeErr := f.Close()
	if writeErr != nil {
		return "", fmt.Errorf("gravar relatório %q: %w", absPath, writeErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("fechar arquivo de relatório %q: %w", absPath, closeErr)
	}

	return absPath, nil
}

// runWith organiza os PDFs de t.opts.InputDir em t.opts.OutputDir, com os
// overrides de dryRun e sample informados (usados pelo fluxo interativo
// para testar a calibração antes de aplicar de verdade). run() é apenas
// runWith(t.opts.DryRun, t.opts.Sample).
func (t *Tool) runWith(dryRun bool, sample int) (tool.Result, error) {
	if strings.TrimSpace(t.opts.InputDir) == "" {
		return tool.Result{}, fmt.Errorf("informe a pasta de entrada (--input)")
	}
	if strings.TrimSpace(t.opts.OutputDir) == "" {
		return tool.Result{}, fmt.Errorf("informe a pasta de destino (--output)")
	}

	// Validado ANTES de qualquer processamento de arquivo, de propósito:
	// falhar por causa de um erro de digitação em --report-format depois de
	// já ter movido ou copiado um lote inteiro seria cruel.
	reportPath := strings.TrimSpace(t.opts.Report)
	reportFormat := ""
	if reportPath != "" {
		f, err := NormalizeReportFormat(t.opts.ReportFormat)
		if err != nil {
			return tool.Result{}, err
		}
		reportFormat = f
	}

	levelSpecs := t.opts.Levels
	if len(t.rawLevels) > 0 {
		specs, err := ParseLevelFlags(t.rawLevels)
		if err != nil {
			return tool.Result{}, err
		}
		levelSpecs = specs
	}

	// Validado ANTES de qualquer processamento de arquivo, pelo mesmo
	// motivo do --report-format acima: --csv e --level são incompatíveis
	// (a hierarquia vem de um ou de outro), e as flags de detalhe de --csv
	// (--csv-key-regex, --csv-key-column, --csv-levels) não têm efeito sem
	// --csv.
	if err := ValidateCSVOptions(t.opts.CSV, t.opts.CSVKeyRegex, t.opts.CSVKeyColumn, t.opts.CSVLevels, len(levelSpecs) > 0); err != nil {
		return tool.Result{}, err
	}

	levels, err := BuildLevels(levelSpecs)
	if err != nil {
		return tool.Result{}, err
	}

	var filenameRegex *regexp.Regexp
	if strings.TrimSpace(t.opts.FilenameRegex) != "" {
		re, err := regexp.Compile(t.opts.FilenameRegex)
		if err != nil {
			return tool.Result{}, fmt.Errorf("regex de nome de arquivo inválida: %w", err)
		}
		filenameRegex = re
	}

	// Carregada ANTES de qualquer processamento de arquivo, pelo mesmo
	// motivo: um erro na planilha (caminho inexistente, coluna que não
	// existe, chave duplicada) só vale a pena descobrir antes de mover ou
	// copiar um lote inteiro.
	var csvMap *pdfutil.CSVMap
	var csvKeyRegex *regexp.Regexp
	var csvWarnings []string
	if csvPath := strings.TrimSpace(t.opts.CSV); csvPath != "" {
		loaded, err := pdfutil.LoadCSVMap(csvPath, strings.TrimSpace(t.opts.CSVKeyColumn), t.opts.CSVLevels)
		if err != nil {
			return tool.Result{}, err
		}
		csvMap = &loaded
		csvWarnings = loaded.Warnings

		re, err := regexp.Compile(strings.TrimSpace(t.opts.CSVKeyRegex))
		if err != nil {
			return tool.Result{}, fmt.Errorf("regex de chave da planilha (--csv-key-regex) inválida: %w", err)
		}
		csvKeyRegex = re
	}

	unclassifiedDir := strings.TrimSpace(t.opts.UnclassifiedDir)
	if unclassifiedDir == "" {
		unclassifiedDir = "sem-classificacao"
	}

	textOpts, err := t.textOptions()
	if err != nil {
		return tool.Result{}, err
	}

	// InputDir/OutputDir absolutos: o manifesto de histórico precisa fazer
	// sentido mesmo que "file-manager undo" rode depois, de um diretório
	// de trabalho diferente do usado nesta organização.
	inputAbs, absErr := filepath.Abs(t.opts.InputDir)
	if absErr != nil {
		inputAbs = t.opts.InputDir
	}
	outputAbs, absErr := filepath.Abs(t.opts.OutputDir)
	if absErr != nil {
		outputAbs = t.opts.OutputDir
	}

	organizeOpts := pdfutil.OrganizeOptions{
		InputDir:        t.opts.InputDir,
		OutputDir:       t.opts.OutputDir,
		Levels:          levels,
		FilenameRegex:   filenameRegex,
		Copy:            !t.opts.Move,
		UnclassifiedDir: unclassifiedDir,
		DryRun:          dryRun,
		Sample:          sample,
		Overwrite:       t.opts.Overwrite,
		Text:            textOpts,
		Recorder:        historyRecorder(inputAbs, outputAbs),
		CSV:             csvMap,
		CSVKeyRegex:     csvKeyRegex,
	}

	result, err := pdfutil.Organize(context.Background(), organizeOpts)
	if err != nil {
		return tool.Result{}, err
	}

	var reportConfirmation string
	if reportPath != "" {
		absReportPath, repErr := writeReportFile(reportPath, reportFormat, result)
		if repErr != nil {
			// Mesma lógica já usada para o manifesto de histórico: a
			// organização já aconteceu de verdade, e falhar (ou fingir que
			// não aconteceu) por causa de um artefato acessório seria pior
			// do que só avisar e seguir.
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"não foi possível gravar o relatório desta execução em %q (a organização já aconteceu normalmente): %v",
				reportPath, repErr,
			))
		} else {
			reportConfirmation = fmt.Sprintf("relatório gravado em %s", absReportPath)
		}
	}

	summary := result.Summary()
	if dryRun {
		summary += " — nada foi copiado ou movido"
	}

	details := make([]string, 0, maxUnclassifiedDetails+2)
	details = append(details, ocrWarnings(textOpts)...)
	// Avisos de células de nível vazias na planilha (--csv): não impedem a
	// leitura (ver LoadCSVMap), mas o usuário precisa saber que algum
	// componente de pasta foi omitido para uma chave específica.
	details = append(details, csvWarnings...)
	details = append(details, result.Warnings...)
	if reportConfirmation != "" {
		details = append(details, reportConfirmation)
	}
	for i, entry := range result.Unclassified {
		if i >= maxUnclassifiedDetails {
			details = append(details, fmt.Sprintf("... e mais %d", len(result.Unclassified)-maxUnclassifiedDetails))
			break
		}
		// pdfutil.UnmatchedReason é a mesma função usada na coluna
		// "motivo" do relatório (--report): fonte única de tradução de
		// Unmatched para texto legível, para que a tela e o relatório
		// nunca divirjam na redação. Em particular, ela NÃO usa o formato
		// "nível %q não encontrado" para Level == "destino" — "destino" é
		// uma pseudo-etiqueta interna (colisão de destino), não um nível
		// calibrado pelo usuário, e misturar os dois formatos confundia
		// quem lia a mensagem.
		details = append(details, fmt.Sprintf("%s: %s", filepath.Base(entry.Source), pdfutil.UnmatchedReason(entry.Unmatched)))
	}

	return tool.Result{Summary: summary, Details: details}, nil
}

// run executa organize-pdf usando t.opts.DryRun e t.opts.Sample (o caminho
// usado pelo comando cobra e por Profile().Apply).
func (t *Tool) run() (tool.Result, error) {
	return t.runWith(t.opts.DryRun, t.opts.Sample)
}
