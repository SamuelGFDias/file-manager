package organizepdf

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

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

	levelSpecs := t.opts.Levels
	if len(t.rawLevels) > 0 {
		specs, err := ParseLevelFlags(t.rawLevels)
		if err != nil {
			return tool.Result{}, err
		}
		levelSpecs = specs
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

	unclassifiedDir := strings.TrimSpace(t.opts.UnclassifiedDir)
	if unclassifiedDir == "" {
		unclassifiedDir = "sem-classificacao"
	}

	textOpts, err := t.textOptions()
	if err != nil {
		return tool.Result{}, err
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
	}

	result, err := pdfutil.Organize(context.Background(), organizeOpts)
	if err != nil {
		return tool.Result{}, err
	}

	summary := result.Summary()
	if dryRun {
		summary += " — nada foi copiado ou movido"
	}

	details := make([]string, 0, maxUnclassifiedDetails+1)
	details = append(details, ocrWarnings(textOpts)...)
	for i, entry := range result.Unclassified {
		if i >= maxUnclassifiedDetails {
			details = append(details, fmt.Sprintf("... e mais %d", len(result.Unclassified)-maxUnclassifiedDetails))
			break
		}
		level := "desconhecido"
		if entry.Unmatched != nil {
			level = entry.Unmatched.Level
		}
		details = append(details, fmt.Sprintf("%s: nível %q não encontrado", filepath.Base(entry.Source), level))
	}

	return tool.Result{Summary: summary, Details: details}, nil
}

// run executa organize-pdf usando t.opts.DryRun e t.opts.Sample (o caminho
// usado pelo comando cobra e por Profile().Apply).
func (t *Tool) run() (tool.Result, error) {
	return t.runWith(t.opts.DryRun, t.opts.Sample)
}
