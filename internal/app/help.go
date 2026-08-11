package app

import "github.com/spf13/cobra"

// newHelpCommand monta o comando "help" com textos em português, no lugar
// do comando padrão que o cobra cria sozinho (Short "Help about any
// command", em inglês — a única peça do programa que fugia do português,
// junto do texto de --help). Ligado ao comando raiz via
// rootCmd.SetHelpCommand em NewRootCommand.
//
// O comportamento reproduz o comando padrão interno do cobra
// (Command.InitDefaultHelpCmd, não exportado): busca o comando pelo
// caminho de argumentos e mostra a ajuda dele; sem argumentos, ou caminho
// que não corresponde a nenhum comando, mostra o uso do comando raiz. A
// única diferença deliberada é que este não tenta propagar o contexto
// (cmd.ctx) do comando "help" para o comando encontrado — esse campo não é
// exportado pelo cobra, e não faz falta aqui: --help nunca executa RunE, só
// imprime texto de ajuda.
func newHelpCommand(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "help [comando]",
		Short: "Mostra ajuda sobre qualquer comando",
		Long: "Mostra ajuda sobre qualquer comando deste programa.\n\n" +
			"Digite \"" + root.Name() + " help [caminho para o comando]\" para ver os detalhes completos.",
		ValidArgsFunction: func(c *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			var completions []string
			cmd, _, err := c.Root().Find(args)
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			if cmd == nil {
				cmd = c.Root()
			}
			for _, subCmd := range cmd.Commands() {
				if subCmd.IsAvailableCommand() || subCmd.Name() == "help" {
					completions = append(completions, subCmd.Name())
				}
			}
			return completions, cobra.ShellCompDirectiveNoFileComp
		},
		Run: func(c *cobra.Command, args []string) {
			cmd, _, err := c.Root().Find(args)
			if cmd == nil || err != nil {
				c.Printf("Tópico de ajuda desconhecido %#q\n", args)
				cobra.CheckErr(c.Root().Usage())
				return
			}

			cmd.InitDefaultHelpFlag()
			cmd.InitDefaultVersionFlag()
			cobra.CheckErr(cmd.Help())
		},
	}
}
