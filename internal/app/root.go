package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"

	"github.com/SamuelGFDias/file-manager/internal/selfupdate"
	"github.com/SamuelGFDias/file-manager/internal/ui"
	"github.com/SamuelGFDias/file-manager/internal/ui/docs"
	"github.com/SamuelGFDias/file-manager/internal/ui/mainmenu"
)

// Version reúne as informações de versão do binário, normalmente
// injetadas em tempo de build via -ldflags -X.
type Version struct {
	// Version é a versão semântica do binário (ex: "0.1.0").
	Version string
	// Commit é o hash curto do commit a partir do qual o binário foi
	// construído.
	Commit string
	// Date é a data/hora de build, em formato RFC3339.
	Date string
}

// String formata a versão como "<versão> (<commit>, <data>)".
func (v Version) String() string {
	return fmt.Sprintf("%s (%s, %s)", v.Version, v.Commit, v.Date)
}

// NewRootCommand monta o comando raiz do cobra com todos os subcomandos:
// um por ferramenta registrada em Tools(), mais "docs" e "version". Quando
// executado sem subcomando, abre o menu interativo (se houver terminal) ou
// devolve um erro claro (se não houver).
func NewRootCommand(v Version) *cobra.Command {
	root := &cobra.Command{
		Use:   "file-manager",
		Short: "Utilitário de linha de comando para manipulação de arquivos",
		Long: "file-manager é um utilitário de linha de comando para manipulação de arquivos, " +
			"com foco em operações sobre PDFs (unir, separar, organizar).\n\n" +
			"Cada operação pode ser executada diretamente como subcomando (ex: " +
			"\"file-manager merge-pdf ...\") ou, quando o binário é chamado sem argumentos em " +
			"um terminal interativo, através de um menu interativo que guia o usuário pelas " +
			"ferramentas disponíveis.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !ui.IsInteractive() {
				return fmt.Errorf(
					"nenhum terminal interativo disponível; rode um subcomando específico " +
						"ou use --help para ver as opções (o menu interativo requer um terminal)",
				)
			}

			nav := ui.NewNavigator()
			return nav.Loop(mainmenu.NewScreen(Tools(), v.String(), v.Version))
		},
	}

	for _, t := range Tools() {
		root.AddCommand(t.Command())
	}

	root.AddCommand(newDocsCommand(v))
	root.AddCommand(newVersionCommand(v))
	root.AddCommand(newUpdateCommand(v))

	return root
}

// newDocsCommand monta o subcomando "docs" e seu subcomando "export".
func newDocsCommand(v Version) *cobra.Command {
	docsCmd := &cobra.Command{
		Use:   "docs",
		Short: "Gera documentação exportável do CLI",
	}

	var format string
	var output string

	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "Exporta a documentação das ferramentas para um arquivo",
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := docs.ParseFormat(format)
			if err != nil {
				return err
			}

			if err := docs.Export(f, output, Tools(), v.String()); err != nil {
				return err
			}

			abs, absErr := filepath.Abs(output)
			if absErr != nil {
				abs = output
			}

			ui.Successf("Documentação exportada em %s", abs)

			return nil
		},
	}

	exportCmd.Flags().StringVarP(&format, "format", "f", "context", "Formato da documentação (\"context\" ou \"skill\")")
	exportCmd.Flags().StringVarP(&output, "output", "o", "", "Caminho do arquivo de saída")
	_ = exportCmd.MarkFlagRequired("output")

	docsCmd.AddCommand(exportCmd)

	return docsCmd
}

// newVersionCommand monta o subcomando "version".
func newVersionCommand(v Version) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Mostra a versão do binário",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println(v.String())
			return nil
		},
	}
}

// newUpdateCommand monta o subcomando "update": consulta o último release
// publicado no GitHub, compara com a versão em execução e, quando
// autorizado, baixa e substitui o próprio executável. É o único caminho de
// atualização para o usuário final, que não acompanha o repositório.
func newUpdateCommand(v Version) *cobra.Command {
	var yes bool
	var checkOnly bool

	cmd := &cobra.Command{
		Use:          "update",
		Short:        "Atualiza o file-manager para a última versão publicada",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			ui.Infof("Consultando o último release publicado em %s...", selfupdate.DefaultRepo)

			release, err := selfupdate.LatestRelease(ctx, selfupdate.DefaultRepo)
			if err != nil {
				return fmt.Errorf(
					"erro ao consultar a última versão publicada; verifique sua conexão com a "+
						"internet e tente novamente: %w", err,
				)
			}

			current := v.Version

			if _, verErr := selfupdate.ParseVersion(current); verErr != nil {
				ui.Warnf(
					"esta é uma compilação local (versão %q), não uma versão publicada oficialmente",
					current,
				)
				ui.Infof("última versão publicada: %s (%s)", release.TagName, release.HTMLURL)
			} else {
				cmp, cmpErr := selfupdate.CompareVersions(current, release.TagName)
				if cmpErr != nil {
					return fmt.Errorf("erro ao comparar versões: %w", cmpErr)
				}

				if cmp >= 0 {
					ui.Successf("você já está na versão mais recente (%s)", current)
					return nil
				}

				ui.Infof("nova versão disponível: %s → %s", current, release.TagName)
				ui.Infof("detalhes: %s", release.HTMLURL)
			}

			if checkOnly {
				return nil
			}

			if !yes {
				confirmed := false
				question := &survey.Confirm{
					Message: fmt.Sprintf("Atualizar para %s agora?", release.TagName),
					Default: false,
				}
				if askErr := survey.AskOne(question, &confirmed); askErr != nil {
					return askErr
				}
				if !confirmed {
					ui.Infof("atualização cancelada")
					return nil
				}
			}

			assetName, err := selfupdate.AssetNameFor(runtime.GOOS, runtime.GOARCH)
			if err != nil {
				return err
			}

			asset, err := selfupdate.FindAsset(release, assetName)
			if err != nil {
				return err
			}

			tmpDir, err := os.MkdirTemp("", "file-manager-update-*")
			if err != nil {
				return fmt.Errorf("erro ao criar diretório temporário para o download: %w", err)
			}
			defer os.RemoveAll(tmpDir)

			tmpBinary := filepath.Join(tmpDir, assetName)

			ui.Infof("baixando %s...", asset.URL)
			if err := selfupdate.Download(ctx, asset.URL, tmpBinary); err != nil {
				return fmt.Errorf("erro ao baixar a nova versão: %w", err)
			}

			ui.Infof("verificando o binário baixado...")
			if err := selfupdate.VerifyBinary(ctx, tmpBinary); err != nil {
				return err
			}

			if err := selfupdate.ReplaceExecutable(tmpBinary); err != nil {
				return err
			}

			ui.Successf("atualizado: %s → %s", current, release.TagName)
			ui.Infof("a nova versão vale a partir da próxima execução do file-manager")

			return nil
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Atualiza sem pedir confirmação")
	cmd.Flags().BoolVar(&checkOnly, "check", false, "Só verifica se há versão nova, sem baixar nem substituir")

	return cmd
}

// Execute é o ponto de entrada chamado pelo main: monta e executa o
// comando raiz, imprime o erro (se houver) uma única vez e devolve o
// código de saída correspondente. Nunca chama os.Exit diretamente, para
// que main permaneça o único lugar que decide sair do processo — o que
// mantém Execute testável.
func Execute(v Version) int {
	if err := NewRootCommand(v).Execute(); err != nil {
		ui.Errorf("%v", err)
		return 1
	}

	return 0
}
