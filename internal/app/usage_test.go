package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// helpOutput executa "<caminho> --help" no comando raiz e devolve o texto
// impresso — via cmd.SetOut/SetArgs, sem depender de terminal virtual nem
// de subprocesso: rápido e determinístico.
func helpOutput(t *testing.T, path ...string) string {
	t.Helper()

	root := NewRootCommand(testVersion())
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(append(append([]string{}, path...), "--help"))

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(%v --help) devolveu erro: %v", path, err)
	}

	return buf.String()
}

// TestRootHelpUsesPortugueseLabels prova a tradução dos rótulos
// estruturais do template de ajuda no comando raiz: "Uso:", "Comandos
// disponíveis:" e "Opções:" precisam aparecer; "Usage:", "Available
// Commands:", "Flags:" e o rodapé em inglês não podem aparecer.
func TestRootHelpUsesPortugueseLabels(t *testing.T) {
	out := helpOutput(t)

	for _, want := range []string{"Uso:", "Comandos disponíveis:", "Opções:"} {
		if !strings.Contains(out, want) {
			t.Errorf("saída de --help não contém %q:\n%s", want, out)
		}
	}

	for _, unwanted := range []string{
		"Usage:",
		"Available Commands:",
		"Flags:",
		"for more information about a command",
	} {
		if strings.Contains(out, unwanted) {
			t.Errorf("saída de --help ainda contém %q (deveria estar traduzido):\n%s", unwanted, out)
		}
	}
}

// TestSubcommandHelpInheritsPortugueseTemplate prova a herança do template:
// "undo" não registra o seu próprio SetUsageTemplate, então precisa herdar
// o template traduzido do comando raiz.
func TestSubcommandHelpInheritsPortugueseTemplate(t *testing.T) {
	out := helpOutput(t, "undo")

	for _, want := range []string{"Uso:", "Opções"} {
		if !strings.Contains(out, want) {
			t.Errorf("saída de \"undo --help\" não contém %q:\n%s", want, out)
		}
	}

	for _, unwanted := range []string{"Usage:", "Flags:"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("saída de \"undo --help\" ainda contém %q (deveria herdar o template traduzido):\n%s", unwanted, out)
		}
	}
}

// TestLeafCommandHelpOmitsEmptyCommandsSection prova que o guarda
// {{if .HasAvailableSubCommands}} do template foi preservado: um comando
// folha (merge-pdf, sem subcomandos) não deve imprimir uma seção
// "Comandos disponíveis:" vazia.
func TestLeafCommandHelpOmitsEmptyCommandsSection(t *testing.T) {
	out := helpOutput(t, "merge-pdf")

	if strings.Contains(out, "Comandos disponíveis:") {
		t.Errorf("saída de \"merge-pdf --help\" (comando sem subcomandos) não deveria conter \"Comandos disponíveis:\":\n%s", out)
	}
}

// TestUsageTemplatePreservesExampleGuard prova que o guarda
// {{if .HasExample}} do template foi preservado: um comando com
// cobra.Command.Example preenchido ainda mostra a seção "Exemplos:".
// Nenhum comando registrado neste CLI usa o campo Example do cobra (os
// exemplos de cada ferramenta vivem em tool.Doc, não em cobra.Command), por
// isso o teste monta um comando sintético para exercitar exatamente esse
// guarda do template.
func TestUsageTemplatePreservesExampleGuard(t *testing.T) {
	cmd := &cobra.Command{
		Use:     "exemplo",
		Short:   "Comando de teste",
		Example: "  file-manager exemplo --flag valor",
		Run:     func(cmd *cobra.Command, args []string) {},
	}
	cmd.SetUsageTemplate(usageTemplatePT)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(--help) devolveu erro: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Exemplos:") {
		t.Errorf("saída de --help de um comando com Example preenchido não contém \"Exemplos:\":\n%s", out)
	}
	if !strings.Contains(out, cmd.Example) {
		t.Errorf("saída de --help não contém o texto do Example:\n%s", out)
	}
}

// TestUsageTemplatePT_FooterUsesCommandPathAndPortuguese garante que o
// rodapé "Use ... para mais informações sobre um comando." usa
// {{.CommandPath}} (não um nome fixo) e está em português, incluindo o
// placeholder "[comando]" (não "[command]").
func TestUsageTemplatePT_FooterUsesCommandPathAndPortuguese(t *testing.T) {
	out := helpOutput(t)

	wantFooter := `Use "file-manager [comando] --help" para mais informações sobre um comando.`
	if !strings.Contains(out, wantFooter) {
		t.Errorf("saída de --help não contém o rodapé esperado %q:\n%s", wantFooter, out)
	}
}
