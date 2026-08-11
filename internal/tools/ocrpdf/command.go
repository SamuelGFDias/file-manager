package ocrpdf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/SamuelGFDias/file-manager/internal/ocr"
	"github.com/SamuelGFDias/file-manager/internal/pdfutil"
	"github.com/SamuelGFDias/file-manager/internal/tool"
	"github.com/SamuelGFDias/file-manager/internal/ui/filepicker"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// params declara os parâmetros aceitos por ocr-pdf. Cada tool.Param
// conecta o mesmo campo de t.opts a uma flag do cobra (BindFlag), a uma
// pergunta interativa (Prompt) e à documentação gerada. --dry-run e
// --report não têm Prompt: a tela interativa (screen.go) sempre roda uma
// simulação antes de aplicar (parte do próprio fluxo, não uma escolha do
// usuário) e não oferece gerar relatório — quem quiser isso usa a flag via
// terminal ou um perfil salvo, como organize-pdf faz com dry_run/sample.
func (t *Tool) params() []tool.Param {
	return []tool.Param{
		{
			Name:        "input",
			Shorthand:   "i",
			Type:        "stringSlice",
			Description: "Arquivo PDF ou pasta a processar. Pode repetir",
			Default:     "",
			Example:     "-i digitalizado.pdf -i ./pasta",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringSliceVarP(&t.opts.Inputs, "input", "i", t.opts.Inputs, "Arquivo PDF ou pasta a processar. Pode repetir")
			},
			Prompt: func() error {
				for {
					var choice string
					if err := survey.AskOne(&survey.Select{
						Message: "Como deseja adicionar entradas para processar?",
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
			Name:        "output-dir",
			Shorthand:   "o",
			Type:        "string",
			Description: "Pasta de saída (vazio = ao lado de cada original)",
			Default:     "",
			Example:     "-o ./pesquisaveis",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVarP(&t.opts.OutputDir, "output-dir", "o", t.opts.OutputDir, "Pasta de saída (vazio = ao lado de cada original)")
			},
			Prompt: func() error {
				var useOutputDir bool
				if err := survey.AskOne(&survey.Confirm{
					Message: "Gravar os PDFs pesquisáveis em uma pasta diferente da original?",
					Default: t.opts.OutputDir != "",
				}, &useOutputDir); err != nil {
					return err
				}
				if !useOutputDir {
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
			Name:        "suffix",
			Shorthand:   "",
			Type:        "string",
			Description: "Sufixo do arquivo gerado",
			Default:     "-ocr",
			Example:     "--suffix _pesquisavel",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVar(&t.opts.Suffix, "suffix", t.opts.Suffix, "Sufixo do arquivo gerado")
			},
			Prompt: func() error {
				return survey.AskOne(&survey.Input{
					Message: "Sufixo do arquivo gerado (ex: nota.pdf -> nota<sufixo>.pdf)",
					Default: t.opts.Suffix,
				}, &t.opts.Suffix)
			},
		},
		{
			Name:        "lang",
			Shorthand:   "",
			Type:        "string",
			Description: `Idioma do OCR (ex: "por", "eng")`,
			Default:     "por",
			Example:     "--lang eng",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVar(&t.opts.Lang, "lang", t.opts.Lang, `Idioma do OCR (ex: "por", "eng")`)
			},
			Prompt: func() error {
				return survey.AskOne(&survey.Input{
					Message: "Idioma do OCR (ex: por, eng)",
					Default: t.opts.Lang,
				}, &t.opts.Lang)
			},
		},
		{
			Name:        "overwrite",
			Shorthand:   "",
			Type:        "bool",
			Description: "Sobrescreve arquivos de saída existentes",
			Default:     "false",
			Example:     "--overwrite",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.BoolVar(&t.opts.Overwrite, "overwrite", t.opts.Overwrite, "Sobrescreve arquivos de saída existentes")
			},
			Prompt: func() error {
				return survey.AskOne(&survey.Confirm{
					Message: "Sobrescrever arquivos de saída que já existirem?",
					Default: t.opts.Overwrite,
				}, &t.opts.Overwrite)
			},
		},
		{
			Name:        "skip-existing",
			Shorthand:   "",
			Type:        "bool",
			Description: "Pula arquivos cuja saída já existe (útil para retomar lote interrompido)",
			Default:     "false",
			Example:     "--skip-existing",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.BoolVar(&t.opts.SkipExisting, "skip-existing", t.opts.SkipExisting, "Pula arquivos cuja saída já existe (útil para retomar lote interrompido)")
			},
			Prompt: func() error {
				return survey.AskOne(&survey.Confirm{
					Message: "Pular (sem erro) arquivos cuja saída já existir? (útil para retomar um lote interrompido)",
					Default: t.opts.SkipExisting,
				}, &t.opts.SkipExisting)
			},
		},
		{
			Name:        "dry-run",
			Shorthand:   "",
			Type:        "bool",
			Description: "Só mostra o que seria feito",
			Default:     "false",
			Example:     "--dry-run",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.BoolVar(&t.opts.DryRun, "dry-run", t.opts.DryRun, "Só mostra o que seria feito")
			},
			Prompt: nil,
		},
		{
			Name:        "report",
			Shorthand:   "",
			Type:        "string",
			Description: "Caminho de um relatório CSV desta execução (vazio = não gera)",
			Default:     "",
			Example:     "--report ./relatorio-ocr.csv",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVar(&t.opts.Report, "report", t.opts.Report, "Caminho de um relatório CSV desta execução (vazio = não gera)")
			},
			Prompt: nil,
		},
	}
}

// formatProgressLine monta a linha de progresso exibida por arquivo, no
// formato "[3/120] nota-003.pdf — 2 página(s)...". Função pura, separada
// de onde é impressa (fmt.Println no comando cobra, ui.Infof na tela
// interativa), para as duas nunca poderem divergir na redação.
func formatProgressLine(done, total int, path string) string {
	pages, err := pdfutil.PageCount(path)
	if err != nil {
		pages = 0
	}
	return fmt.Sprintf("[%d/%d] %s — %d página(s)...", done, total, filepath.Base(path), pages)
}

// cliProgress é o Progress usado pelo comando cobra: imprime a linha de
// progresso diretamente em stdout, em tempo real — indispensável num lote
// grande (~1s por página; 200 documentos de 3 páginas levam quase 10
// minutos), para que o usuário não conclua que o processo travou.
func cliProgress(done, total int, path string) {
	fmt.Println(formatProgressLine(done, total, path))
}

// Command devolve o subcomando cobra de ocr-pdf.
func (t *Tool) Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ocr-pdf",
		Short: "Torna PDFs digitalizados pesquisáveis, gravando a camada de texto no arquivo",
		Long: "Transforma PDFs digitalizados (imagem, sem camada de texto) em PDFs pesquisáveis de verdade: " +
			"o texto reconhecido por OCR é gravado de volta no arquivo, em vez de só usado em memória para " +
			"casar uma expressão regular (como organize-pdf/split-pdf fazem). Só processa arquivos cujas " +
			"páginas sejam TODAS puro scan (uma única imagem, sem texto embutido); qualquer página mista " +
			"(imagem + texto, ou mais de uma imagem) faz o arquivo inteiro ser recusado, com o motivo " +
			"explicado, para nunca perder conteúdo em silêncio. Exige o Tesseract instalado.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := t.runWith(t.opts.DryRun, cliProgress)
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
	_ = cmd.RegisterFlagCompletionFunc("lang", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return ocr.CompletionLanguages(), cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

// writeOCRizeReportFile monta as linhas do relatório a partir de result e
// grava em CSV no caminho path (criando diretórios intermediários que
// faltarem). Devolve o caminho absoluto gravado. Mesmo padrão de
// writeReportFile em organize-pdf: erros aqui NUNCA falham runWith — o
// processamento já aconteceu de verdade.
func writeOCRizeReportFile(path string, result pdfutil.OCRizeResult) (string, error) {
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

	rows := pdfutil.BuildOCRizeReport(result)
	writeErr := pdfutil.WriteOCRizeReportCSV(f, rows)

	closeErr := f.Close()
	if writeErr != nil {
		return "", fmt.Errorf("gravar relatório %q: %w", absPath, writeErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("fechar arquivo de relatório %q: %w", absPath, closeErr)
	}

	return absPath, nil
}

// ocrizeRaw executa ocr-pdf usando t.opts, com o override de dryRun
// informado (usado pela tela interativa para simular antes de aplicar de
// verdade — mesmo princípio de dry-run compartilhado já usado por
// organize-pdf) e a função de progresso informada (diferente entre o
// comando cobra, que imprime em stdout, e a tela interativa, que usa
// ui.Infof). Devolve o pdfutil.OCRizeResult "cru", além do tool.Result já
// formatado — a tela interativa precisa do resultado cru para decidir se
// há algo elegível a aplicar (ver screen.go), o que tool.Result (só
// strings) não permite inspecionar.
func (t *Tool) ocrizeRaw(dryRun bool, progress func(done, total int, path string)) (pdfutil.OCRizeResult, tool.Result, error) {
	if len(t.opts.Inputs) == 0 {
		return pdfutil.OCRizeResult{}, tool.Result{}, fmt.Errorf("informe ao menos um arquivo ou pasta em --input")
	}

	// ocr-pdf EXIGE o Tesseract — ao contrário do OCR usado como
	// fallback de leitura em organize-pdf/split-pdf (que degrada
	// normalmente sem ele), aqui é o próprio propósito da ferramenta.
	// Falha ANTES de processar qualquer coisa, inclusive em --dry-run:
	// simular sem o motor disponível daria uma falsa promessa de que a
	// execução real funcionaria.
	engine := ocr.NewTesseract()
	if !engine.Available() {
		return pdfutil.OCRizeResult{}, tool.Result{}, fmt.Errorf("ocr-pdf exige o Tesseract instalado no sistema. %s", ocr.InstallHint())
	}

	lang := strings.TrimSpace(t.opts.Lang)
	if lang == "" {
		lang = "por"
	}

	var warnings []string
	if !engine.HasLanguage(lang) {
		warnings = append(warnings, fmt.Sprintf(
			"aviso: pacote de idioma %q do Tesseract não está instalado — o reconhecimento vai cair no idioma padrão e a precisão tende a ser baixa",
			lang,
		))
	}

	suffix := t.opts.Suffix
	if suffix == "" {
		suffix = "-ocr"
	}

	reportPath := strings.TrimSpace(t.opts.Report)

	result, err := pdfutil.OCRize(context.Background(), pdfutil.OCRizeOptions{
		Inputs:       t.opts.Inputs,
		MaxDepth:     t.opts.MaxDepth,
		OutputDir:    t.opts.OutputDir,
		Suffix:       suffix,
		Lang:         lang,
		Overwrite:    t.opts.Overwrite,
		SkipExisting: t.opts.SkipExisting,
		DryRun:       dryRun,
		Engine:       engine,
		Progress:     progress,
	})
	if err != nil {
		return pdfutil.OCRizeResult{}, tool.Result{}, err
	}

	var reportConfirmation string
	if reportPath != "" {
		absReportPath, repErr := writeOCRizeReportFile(reportPath, result)
		if repErr != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"não foi possível gravar o relatório desta execução em %q (o processamento já aconteceu normalmente): %v",
				reportPath, repErr,
			))
		} else {
			reportConfirmation = fmt.Sprintf("relatório gravado em %s", absReportPath)
		}
	}

	summary := result.Summary()

	details := make([]string, 0, len(result.Processed)+len(result.Skipped)+len(warnings)+2)
	details = append(details, warnings...)
	details = append(details, result.Warnings...)
	if reportConfirmation != "" {
		details = append(details, reportConfirmation)
	}
	for _, entry := range result.Skipped {
		details = append(details, fmt.Sprintf("pulado: %s: %s", filepath.Base(entry.Source), entry.Reason))
	}
	for _, entry := range result.Processed {
		verb := "processado"
		if dryRun {
			verb = "seria processado"
		}
		details = append(details, fmt.Sprintf("%s: %s (%s, %d página(s))", filepath.Base(entry.Source), verb, entry.Dest, entry.Pages))
	}

	return result, tool.Result{Summary: summary, Details: details}, nil
}

// runWith executa ocr-pdf e devolve só o tool.Result, formatado — usado
// pelo comando cobra, que não precisa inspecionar o resultado cru.
func (t *Tool) runWith(dryRun bool, progress func(done, total int, path string)) (tool.Result, error) {
	_, result, err := t.ocrizeRaw(dryRun, progress)
	return result, err
}

// run executa ocr-pdf usando t.opts.DryRun e imprimindo progresso em
// stdout — o caminho usado por Profile().Apply.
func (t *Tool) run() (tool.Result, error) {
	return t.runWith(t.opts.DryRun, cliProgress)
}
