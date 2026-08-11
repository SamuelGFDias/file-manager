package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"

	"github.com/SamuelGFDias/file-manager/internal/config"
	"github.com/SamuelGFDias/file-manager/internal/selfupdate"
	"github.com/SamuelGFDias/file-manager/internal/tool"
	"github.com/SamuelGFDias/file-manager/internal/ui"
	"github.com/SamuelGFDias/file-manager/internal/ui/docs"
	"github.com/SamuelGFDias/file-manager/internal/ui/mainmenu"
	"github.com/SamuelGFDias/file-manager/internal/ui/profiles"
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
	root.AddCommand(newProfilesCommand())
	root.AddCommand(newUndoCommand())

	return root
}

// newProfilesCommand monta o comando pai "profiles" e seus quatro
// subcomandos (list, export, import, path). É o caminho de linha de comando
// para o fluxo "calibrar numa máquina, usar em outra": a tela interativa
// (internal/ui/profiles) cobre o mesmo CRUD para quem prefere navegar por
// menu, mas exportar/importar por arquivo também precisa funcionar sem
// terminal interativo (ex: dentro de um script).
func newProfilesCommand() *cobra.Command {
	profilesCmd := &cobra.Command{
		Use:   "profiles",
		Short: "Gerencia perfis salvos das ferramentas",
		Long: "Gerencia perfis salvos das ferramentas: listar, exportar para um arquivo, " +
			"importar de um arquivo recebido de outra pessoa, e localizar o diretório onde " +
			"ficam guardados.",
	}

	profilesCmd.AddCommand(newProfilesListCommand())
	profilesCmd.AddCommand(newProfilesExportCommand())
	profilesCmd.AddCommand(newProfilesImportCommand())
	profilesCmd.AddCommand(newProfilesPathCommand())

	return profilesCmd
}

// newProfilesListCommand monta o subcomando "profiles list". Sem --tool,
// lista os perfis de todas as ferramentas que suportam perfis, agrupados
// por ferramenta.
func newProfilesListCommand() *cobra.Command {
	var toolID string

	cmd := &cobra.Command{
		Use:          "list",
		Short:        "Lista os perfis salvos",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			supported := profiles.SupportingTools(Tools())

			if toolID != "" {
				var match tool.Tool
				for _, t := range supported {
					if t.Meta().ID == toolID {
						match = t
						break
					}
				}
				if match == nil {
					return fmt.Errorf("ferramenta %q não existe ou não suporta perfis", toolID)
				}
				supported = []tool.Tool{match}
			}

			if len(supported) == 0 {
				ui.Infof("Nenhuma ferramenta deste CLI suporta perfis salvos.")
				return nil
			}

			foundAny := false
			for _, t := range supported {
				names, err := config.List(t.Meta().ID)
				if err != nil {
					return fmt.Errorf("erro ao listar perfis de %q: %w", t.Meta().ID, err)
				}
				if len(names) == 0 {
					continue
				}

				foundAny = true
				ui.Infof("%s:", ui.Bold(t.Meta().Title))
				for _, name := range names {
					path, err := config.ProfilePath(t.Meta().ID, name)
					if err != nil {
						return err
					}
					ui.Infof("  %s — %s", name, ui.PathText(path))
				}
			}

			if !foundAny {
				ui.Infof("Nenhum perfil salvo encontrado.")
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&toolID, "tool", "", "Filtra por ID da ferramenta")

	return cmd
}

// newProfilesExportCommand monta o subcomando "profiles export".
func newProfilesExportCommand() *cobra.Command {
	var toolID, name, output string

	cmd := &cobra.Command{
		Use:          "export",
		Short:        "Exporta um perfil salvo para um arquivo",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.ExportProfile(toolID, name, output); err != nil {
				return err
			}

			abs, absErr := filepath.Abs(output)
			if absErr != nil {
				abs = output
			}

			ui.Successf("Perfil %q exportado para %s", name, abs)
			return nil
		},
	}

	cmd.Flags().StringVar(&toolID, "tool", "", "ID da ferramenta dona do perfil")
	cmd.Flags().StringVar(&name, "name", "", "Nome do perfil a exportar")
	cmd.Flags().StringVar(&output, "output", "", "Caminho do arquivo de saída")
	_ = cmd.MarkFlagRequired("tool")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("output")

	return cmd
}

// newProfilesImportCommand monta o subcomando "profiles import". Valida que
// a ferramenta declarada no arquivo existe e suporta perfis, e que o
// conteúdo de "data" decodifica na struct de Options dessa ferramenta —
// um arquivo corrompido ou de versão incompatível falha aqui, na
// importação, em vez de falhar mais tarde ao tentar usar o perfil.
func newProfilesImportCommand() *cobra.Command {
	var file, name string
	var force bool

	cmd := &cobra.Command{
		Use:          "import",
		Short:        "Importa um perfil de um arquivo",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			imported, err := config.ReadProfileFile(file)
			if err != nil {
				return err
			}

			var match tool.Tool
			for _, t := range Tools() {
				if t.Meta().ID == imported.Tool {
					match = t
					break
				}
			}
			if match == nil {
				return fmt.Errorf(
					"o arquivo %q referencia a ferramenta %q, que não existe neste CLI",
					file, imported.Tool,
				)
			}
			if match.Profile() == nil {
				return fmt.Errorf("a ferramenta %q não suporta perfis salvos", imported.Tool)
			}

			target := imported.Name
			if name != "" {
				target = name
			}
			if err := config.ValidateName(target); err != nil {
				return fmt.Errorf("nome de destino inválido: %w", err)
			}

			empty := match.Profile().Empty()
			if err := imported.Node.Decode(empty); err != nil {
				return config.DecodeError(imported.Tool, file, err)
			}

			if err := config.ImportProfile(imported, target, force); err != nil {
				return err
			}

			ui.Successf("Perfil %q importado com sucesso para a ferramenta %q.", target, imported.Tool)
			return nil
		},
	}

	cmd.Flags().StringVar(&file, "file", "", "Caminho do arquivo de perfil a importar")
	cmd.Flags().StringVar(&name, "name", "", "Sobrescreve o nome do perfil importado")
	cmd.Flags().BoolVar(&force, "force", false, "Sobrescreve um perfil existente com o mesmo nome")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

// newProfilesPathCommand monta o subcomando "profiles path": imprime o
// diretório onde os perfis são guardados, para o usuário achar os arquivos
// sem precisar saber de os.UserConfigDir.
func newProfilesPathCommand() *cobra.Command {
	return &cobra.Command{
		Use:          "path",
		Short:        "Mostra o diretório onde os perfis são guardados",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			base, err := config.BaseDir()
			if err != nil {
				return err
			}
			fmt.Println(filepath.Join(base, "profiles"))
			return nil
		},
	}
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
