package splitpdf

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/SamuelGFDias/file-manager/internal/pdfutil"
	"github.com/SamuelGFDias/file-manager/internal/tool"
	"github.com/SamuelGFDias/file-manager/internal/ui"
	"github.com/SamuelGFDias/file-manager/internal/ui/filepicker"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// modeOrder define a ordem estável em que os modos aparecem no select
// interativo.
var modeOrder = []string{"page", "range", "regex"}

// modeLabels traduz cada valor interno de Mode para o rótulo em português
// mostrado no select interativo.
var modeLabels = map[string]string{
	"page":  "Uma página por arquivo",
	"range": "Por intervalos de páginas",
	"regex": "Por expressão regular no conteúdo",
}

// params declara, em um único lugar, todos os parâmetros aceitos pela
// ferramenta split-pdf. Cada tool.Param liga o mesmo campo de t.opts a uma
// flag do cobra (BindFlag), a uma pergunta interativa (Prompt) e à
// documentação gerada (via tool.DocFlags).
func (t *Tool) params() []tool.Param {
	return []tool.Param{
		{
			Name:        "input",
			Shorthand:   "i",
			Type:        "string",
			Description: "PDF a separar",
			Default:     "",
			Example:     "documento.pdf",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVarP(&t.opts.Input, "input", "i", t.opts.Input, "PDF a separar")
			},
			Prompt: func() error {
				path, err := filepicker.PickFile(".", []string{".pdf"})
				if err != nil {
					return err
				}
				t.opts.Input = path
				return nil
			},
		},
		{
			Name:        "output-dir",
			Shorthand:   "o",
			Type:        "string",
			Description: "Pasta de saída (vazio = mesma pasta do input)",
			Default:     "",
			Example:     "./saida",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVarP(&t.opts.OutputDir, "output-dir", "o", t.opts.OutputDir, "Pasta de saída (vazio = mesma pasta do input)")
			},
			Prompt: func() error {
				same := true
				if err := survey.AskOne(&survey.Confirm{
					Message: "Salvar na mesma pasta do arquivo original?",
					Default: true,
				}, &same); err != nil {
					return err
				}
				if same {
					t.opts.OutputDir = ""
					return nil
				}
				dir, err := filepicker.PickDir(".")
				if err != nil {
					return err
				}
				t.opts.OutputDir = dir
				return nil
			},
		},
		{
			Name:        "mode",
			Shorthand:   "",
			Type:        "string",
			Description: "Critério de separação: page, range ou regex",
			Default:     "page",
			Example:     "range",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVar(&t.opts.Mode, "mode", t.opts.Mode, "Critério de separação: page, range ou regex")
			},
			Prompt: func() error {
				options := make([]string, 0, len(modeOrder))
				for _, m := range modeOrder {
					options = append(options, modeLabels[m])
				}
				def := modeLabels[t.opts.Mode]
				if def == "" {
					def = modeLabels["page"]
				}
				selected := ""
				if err := survey.AskOne(&survey.Select{
					Message: "Como deseja separar o PDF?",
					Options: options,
					Default: def,
				}, &selected); err != nil {
					return err
				}
				for _, m := range modeOrder {
					if modeLabels[m] == selected {
						t.opts.Mode = m
						break
					}
				}
				return nil
			},
		},
		{
			Name:        "ranges",
			Shorthand:   "",
			Type:        "string",
			Description: "Intervalos de páginas, ex 1-5,6-10,11- (só no modo range)",
			Default:     "",
			Example:     "1-5,6-10,11-",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVar(&t.opts.Ranges, "ranges", t.opts.Ranges, "Intervalos de páginas, ex 1-5,6-10,11- (só no modo range)")
			},
			Prompt: func() error {
				if t.opts.Mode != "range" {
					return nil
				}
				return survey.AskOne(&survey.Input{
					Message: "Informe os intervalos de páginas (ex: 1-5,6-10,11-):",
					Default: t.opts.Ranges,
				}, &t.opts.Ranges)
			},
		},
		{
			Name:        "regex",
			Shorthand:   "",
			Type:        "string",
			Description: "Regex aplicada ao texto de cada página (só no modo regex)",
			Default:     "",
			Example:     `NF:\s*(\d+)`,
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVar(&t.opts.Regex, "regex", t.opts.Regex, "Regex aplicada ao texto de cada página (só no modo regex)")
			},
			Prompt: func() error {
				if t.opts.Mode != "regex" {
					return nil
				}
				const maxAttempts = 3
				value := t.opts.Regex
				for attempt := 1; attempt <= maxAttempts; attempt++ {
					if err := survey.AskOne(&survey.Input{
						Message: `Informe a expressão regular aplicada ao texto de cada página. Um grupo de captura entre parênteses vira o nome do arquivo (ex: "NF:\s*(\d+)" nomeia os arquivos pelo número da nota):`,
						Default: value,
					}, &value); err != nil {
						return err
					}
					if _, err := regexp.Compile(value); err != nil {
						ui.Warnf("expressão regular inválida: %v", err)
						if attempt == maxAttempts {
							return fmt.Errorf("expressão regular inválida após %d tentativas: %w", maxAttempts, err)
						}
						continue
					}
					t.opts.Regex = value
					return nil
				}
				return nil
			},
		},
		{
			Name:        "name-template",
			Shorthand:   "",
			Type:        "string",
			Description: "Template do nome do arquivo quando não há captura da regex",
			Default:     "pagina-%03d.pdf",
			Example:     "pagina-%03d.pdf",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVar(&t.opts.NameTemplate, "name-template", t.opts.NameTemplate, "Template do nome do arquivo quando não há captura da regex")
			},
			Prompt: nil,
		},
		{
			Name:        "overwrite",
			Shorthand:   "",
			Type:        "bool",
			Description: "Sobrescreve arquivos de saída existentes",
			Default:     "false",
			Example:     "true",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.BoolVar(&t.opts.Overwrite, "overwrite", t.opts.Overwrite, "Sobrescreve arquivos de saída existentes")
			},
			Prompt: nil,
		},
	}
}

// Command devolve o subcomando cobra "split-pdf".
func (t *Tool) Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "split-pdf",
		Short:        "Separa um PDF em vários arquivos",
		Long:         "Separa um PDF em vários arquivos por página, por intervalos de páginas ou por uma expressão regular aplicada ao texto de cada página.",
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

	return cmd
}

// run executa a separação do PDF de acordo com t.opts e devolve o
// resultado.
func (t *Tool) run() (tool.Result, error) {
	if t.opts.Input == "" {
		return tool.Result{}, fmt.Errorf("informe o arquivo de entrada (--input)")
	}

	var mode pdfutil.SplitMode
	switch t.opts.Mode {
	case "page":
		mode = pdfutil.SplitByPage
	case "range":
		mode = pdfutil.SplitByRange
	case "regex":
		mode = pdfutil.SplitByRegex
	default:
		return tool.Result{}, fmt.Errorf("modo de separação inválido: %q (valores válidos: page, range, regex)", t.opts.Mode)
	}

	opts := pdfutil.SplitOptions{
		Input:        t.opts.Input,
		OutputDir:    t.opts.OutputDir,
		Mode:         mode,
		NameTemplate: t.opts.NameTemplate,
		Overwrite:    t.opts.Overwrite,
	}

	switch mode {
	case pdfutil.SplitByRange:
		if strings.TrimSpace(t.opts.Ranges) == "" {
			return tool.Result{}, fmt.Errorf("modo range requer --ranges (ex: 1-5,6-10,11-)")
		}
		ranges, err := pdfutil.ParseRanges(t.opts.Ranges)
		if err != nil {
			return tool.Result{}, err
		}
		opts.Ranges = ranges

	case pdfutil.SplitByRegex:
		if strings.TrimSpace(t.opts.Regex) == "" {
			return tool.Result{}, fmt.Errorf("modo regex requer --regex")
		}
		re, err := regexp.Compile(t.opts.Regex)
		if err != nil {
			return tool.Result{}, fmt.Errorf("expressão regular inválida: %w", err)
		}
		opts.Regex = re
	}

	result, err := pdfutil.Split(context.Background(), opts)
	if err != nil {
		return tool.Result{}, err
	}

	outDir := t.opts.OutputDir
	if outDir == "" {
		outDir = filepath.Dir(t.opts.Input)
	}

	details := make([]string, 0, len(result.Outputs)+len(result.Warnings))
	details = append(details, result.Outputs...)
	for _, w := range result.Warnings {
		details = append(details, "aviso: "+w)
	}

	return tool.Result{
		Summary: fmt.Sprintf("%d arquivos gerados em %s", len(result.Outputs), outDir),
		Details: details,
	}, nil
}
