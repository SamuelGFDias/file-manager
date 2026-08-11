package mergepdf

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/SamuelGFDias/file-manager/internal/pdfutil"
	"github.com/SamuelGFDias/file-manager/internal/tool"
	"github.com/SamuelGFDias/file-manager/internal/ui/filepicker"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// params declara os parâmetros aceitos por merge-pdf. Cada tool.Param
// conecta o mesmo campo de t.opts a uma flag do cobra (BindFlag), a uma
// pergunta interativa (Prompt) e à documentação gerada.
func (t *Tool) params() []tool.Param {
	return []tool.Param{
		{
			Name:        "input",
			Shorthand:   "i",
			Type:        "stringSlice",
			Description: "Arquivo PDF ou pasta a incluir. Pode repetir.",
			Default:     "",
			Example:     "-i arquivo.pdf -i ./pasta",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringSliceVarP(&t.opts.Inputs, "input", "i", t.opts.Inputs, "Arquivo PDF ou pasta a incluir. Pode repetir.")
			},
			Prompt: func() error {
				for {
					var choice string
					if err := survey.AskOne(&survey.Select{
						Message: "Como deseja adicionar entradas para unir?",
						Options: []string{"Escolher arquivos específicos", "Incluir uma pasta inteira"},
					}, &choice); err != nil {
						return err
					}

					switch choice {
					case "Escolher arquivos específicos":
						files, err := filepicker.PickFiles(".", []string{".pdf"})
						if err != nil {
							return err
						}
						t.opts.Inputs = append(t.opts.Inputs, files...)
					case "Incluir uma pasta inteira":
						dir, err := filepicker.PickDir(".")
						if err != nil {
							return err
						}
						t.opts.Inputs = append(t.opts.Inputs, dir)

						var depthChoice string
						if err := survey.AskOne(&survey.Select{
							Message: "Profundidade de varredura da pasta",
							Options: []string{
								"Só esta pasta (0)",
								"Esta pasta e subpastas diretas (1)",
								"Todos os níveis (-1)",
								"Personalizado",
							},
						}, &depthChoice); err != nil {
							return err
						}

						switch depthChoice {
						case "Só esta pasta (0)":
							t.opts.MaxDepth = 0
						case "Esta pasta e subpastas diretas (1)":
							t.opts.MaxDepth = 1
						case "Todos os níveis (-1)":
							t.opts.MaxDepth = -1
						case "Personalizado":
							var raw string
							if err := survey.AskOne(&survey.Input{
								Message: "Profundidade personalizada (número inteiro; -1 para ilimitado)",
							}, &raw); err != nil {
								return err
							}
							depth, err := strconv.Atoi(strings.TrimSpace(raw))
							if err != nil {
								return fmt.Errorf("profundidade inválida %q: %w", raw, err)
							}
							t.opts.MaxDepth = depth
						}
					}

					var more bool
					if err := survey.AskOne(&survey.Confirm{
						Message: "Adicionar mais arquivos ou pastas?",
						Default: false,
					}, &more); err != nil {
						return err
					}
					if !more {
						break
					}
				}

				return nil
			},
		},
		{
			Name:        "max-depth",
			Shorthand:   "",
			Type:        "int",
			Description: "Profundidade ao varrer pastas: 0 = só a pasta, N = N níveis, -1 = ilimitado",
			Default:     "1",
			Example:     "--max-depth -1",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.IntVar(&t.opts.MaxDepth, "max-depth", t.opts.MaxDepth, "Profundidade ao varrer pastas: 0 = só a pasta, N = N níveis, -1 = ilimitado")
			},
			Prompt: nil,
		},
		{
			Name:        "output",
			Shorthand:   "o",
			Type:        "string",
			Description: "Caminho do PDF resultante",
			Default:     "",
			Example:     "-o ./unido.pdf",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVarP(&t.opts.Output, "output", "o", t.opts.Output, "Caminho do PDF resultante")
			},
			Prompt: func() error {
				if t.opts.Output == "" {
					t.opts.Output = "./unido.pdf"
				}
				if err := survey.AskOne(&survey.Input{
					Message: "Caminho do PDF resultante",
					Default: t.opts.Output,
				}, &t.opts.Output); err != nil {
					return err
				}

				if _, err := os.Stat(t.opts.Output); err == nil {
					return survey.AskOne(&survey.Confirm{
						Message: fmt.Sprintf("O arquivo %q já existe. Sobrescrever?", t.opts.Output),
						Default: t.opts.Overwrite,
					}, &t.opts.Overwrite)
				}

				return nil
			},
		},
		{
			Name:        "sort",
			Shorthand:   "",
			Type:        "string",
			Description: "Ordem dos arquivos: \"name\" ou \"mtime\"",
			Default:     "name",
			Example:     "--sort mtime",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVar(&t.opts.Sort, "sort", t.opts.Sort, "Ordem dos arquivos: \"name\" ou \"mtime\"")
			},
			Prompt: nil,
		},
		{
			Name:        "overwrite",
			Shorthand:   "",
			Type:        "bool",
			Description: "Sobrescreve o arquivo de saída se já existir",
			Default:     "false",
			Example:     "--overwrite",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.BoolVar(&t.opts.Overwrite, "overwrite", t.opts.Overwrite, "Sobrescreve o arquivo de saída se já existir")
			},
			Prompt: nil,
		},
	}
}

// Command devolve o subcomando cobra de merge-pdf.
func (t *Tool) Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "merge-pdf",
		Short:        "Une vários PDFs em um único arquivo",
		Long:         "Une arquivos e/ou pastas contendo PDFs em um único arquivo de saída, na ordem definida por --sort. Pastas são varridas conforme --max-depth.",
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

// run executa merge-pdf usando t.opts e devolve o resultado.
func (t *Tool) run() (tool.Result, error) {
	if len(t.opts.Inputs) == 0 {
		return tool.Result{}, fmt.Errorf("informe ao menos um arquivo ou pasta em --input")
	}
	if strings.TrimSpace(t.opts.Output) == "" {
		return tool.Result{}, fmt.Errorf("informe o caminho de saída em --output")
	}

	result, err := pdfutil.Merge(context.Background(), pdfutil.MergeOptions{
		Inputs:    t.opts.Inputs,
		MaxDepth:  t.opts.MaxDepth,
		Output:    t.opts.Output,
		Sort:      t.opts.Sort,
		Overwrite: t.opts.Overwrite,
	})
	if err != nil {
		return tool.Result{}, err
	}

	return tool.Result{
		Summary: fmt.Sprintf("%d PDFs unidos em %s (%d páginas)", len(result.Files), result.Output, result.PageCount),
		Details: result.Files,
	}, nil
}
