package app

// usageTemplatePT é uma cópia adaptada do template padrão do cobra
// (spf13/cobra v1.10.2, command.go, const defaultUsageTemplate) com os
// RÓTULOS ESTRUTURAIS traduzidos para português — nada mais. Toda a lógica
// de template (os `{{if ...}}`, `{{range ...}}`, os guardas como
// `.HasAvailableSubCommands`/`.HasAvailableLocalFlags`/`.HasHelpSubCommands`,
// os `rpad`, o `trimTrailingWhitespaces`) foi preservada byte a byte na
// mesma ordem e estrutura do original: só o texto literal entre os
// marcadores de ação mudou. Isso importa porque um guarda removido por
// engano faz a ajuda de um comando sem subcomandos (ou sem flags, ou sem
// exemplos) imprimir uma seção vazia — o motivo pelo qual o prompt original
// deste projeto desaconselhava reescrever o template à mão, e o motivo pelo
// qual esta versão foi feita por cópia adaptada, não por reescrita livre.
//
// Rótulos traduzidos: "Usage" -> "Uso", "Aliases" -> "Apelidos", "Examples"
// -> "Exemplos", "Available Commands" -> "Comandos disponíveis",
// "Additional Commands" -> "Comandos adicionais", "Flags" -> "Opções",
// "Global Flags" -> "Opções globais", "Additional help topics" -> "Tópicos
// de ajuda adicionais", e o rodapé 'Use "..." for more information about a
// command.' -> 'Use "..." para mais informações sobre um comando.'
// (mantendo `{{.CommandPath}}`, não um nome fixo). O placeholder genérico
// "[command]" também virou "[comando]" nos dois lugares em que aparece
// (linha de Uso e rodapé), pela mesma razão: metade traduzido lê pior que
// nada traduzido, e por ser a saída mais vista do programa.
//
// Aplicado em NewRootCommand via root.SetUsageTemplate(usageTemplatePT). O
// cobra propaga o template do pai para um comando filho que não tem template
// próprio (Command.UsageTemplate, quando c.usageTemplate == "", devolve
// c.parent.UsageTemplate() recursivamente) — nenhum subcomando precisa
// (nem deve) registrar o seu próprio.
//
// Risco aceito e documentado: uma atualização futura da dependência cobra
// que mude defaultUsageTemplate não propaga automaticamente para esta
// cópia. Degradação no pior caso é branda — a ajuda deixa de refletir uma
// seção nova da biblioteca — nunca quebra a execução. Reavaliar esta cópia
// sempre que o go.mod atualizar a versão de spf13/cobra.
const usageTemplatePT = `Uso:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [comando]{{end}}{{if gt (len .Aliases) 0}}

Apelidos:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Exemplos:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Comandos disponíveis:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

Comandos adicionais:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Opções:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Opções globais:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Tópicos de ajuda adicionais:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [comando] --help" para mais informações sobre um comando.{{end}}
`
