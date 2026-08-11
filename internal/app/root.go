package app

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

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
			return nav.Loop(mainmenu.NewScreen(Tools(), v.String()))
		},
	}

	for _, t := range Tools() {
		root.AddCommand(t.Command())
	}

	root.AddCommand(newDocsCommand(v))
	root.AddCommand(newVersionCommand(v))

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
