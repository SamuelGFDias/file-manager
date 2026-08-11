package app

import (
	"errors"
	"flag"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestTools(t *testing.T) {
	tools := Tools()

	if len(tools) != 3 {
		t.Fatalf("Tools() devolveu %d ferramentas, esperava 3", len(tools))
	}

	wantIDs := []string{"merge-pdf", "split-pdf", "organize-pdf"}
	gotIDs := make([]string, len(tools))
	for i, tl := range tools {
		gotIDs[i] = tl.Meta().ID
	}

	for i, want := range wantIDs {
		if gotIDs[i] != want {
			t.Errorf("Tools()[%d].Meta().ID = %q, esperava %q", i, gotIDs[i], want)
		}
	}
}

func TestToolsUniqueIDs(t *testing.T) {
	tools := Tools()

	seen := make(map[string]bool, len(tools))
	for _, tl := range tools {
		id := tl.Meta().ID
		if seen[id] {
			t.Fatalf("ID de ferramenta duplicado: %q", id)
		}
		seen[id] = true
	}
}

func TestNewRootCommandUse(t *testing.T) {
	cmd := NewRootCommand(Version{Version: "0.1.0", Commit: "abc1234", Date: "2026-08-11T12:00:00Z"})

	if cmd.Use != "file-manager" {
		t.Errorf("cmd.Use = %q, esperava %q", cmd.Use, "file-manager")
	}
}

func TestNewRootCommandHasSubcommands(t *testing.T) {
	cmd := NewRootCommand(Version{Version: "0.1.0", Commit: "abc1234", Date: "2026-08-11T12:00:00Z"})

	names := make(map[string]bool)
	for _, c := range cmd.Commands() {
		names[c.Name()] = true
	}

	wantNames := []string{"merge-pdf", "split-pdf", "organize-pdf", "docs", "version", "update"}
	for _, want := range wantNames {
		if !names[want] {
			t.Errorf("subcomando %q não encontrado; subcomandos presentes: %v", want, names)
		}
	}
}

func TestVersionString(t *testing.T) {
	v := Version{Version: "0.1.0", Commit: "abc1234", Date: "2026-08-11T12:00:00Z"}
	s := v.String()

	for _, part := range []string{v.Version, v.Commit, v.Date} {
		if !strings.Contains(s, part) {
			t.Errorf("Version.String() = %q, esperava que contivesse %q", s, part)
		}
	}
}

func TestDocsExportCommandFlags(t *testing.T) {
	cmd := NewRootCommand(Version{Version: "0.1.0", Commit: "abc1234", Date: "2026-08-11T12:00:00Z"})

	var found bool
	for _, c := range cmd.Commands() {
		if c.Name() != "docs" {
			continue
		}
		for _, sub := range c.Commands() {
			if sub.Name() != "export" {
				continue
			}
			found = true
			if sub.Flags().Lookup("format") == nil {
				t.Errorf("subcomando docs export não tem a flag --format")
			}
			if sub.Flags().Lookup("output") == nil {
				t.Errorf("subcomando docs export não tem a flag --output")
			}
		}
	}

	if !found {
		t.Fatalf("subcomando \"docs export\" não encontrado")
	}
}

func TestUpdateCommandFlags(t *testing.T) {
	cmd := NewRootCommand(Version{Version: "0.1.0", Commit: "abc1234", Date: "2026-08-11T12:00:00Z"})

	var update *cobra.Command
	for _, c := range cmd.Commands() {
		if c.Name() == "update" {
			update = c
			break
		}
	}

	if update == nil {
		t.Fatalf("subcomando \"update\" não encontrado")
	}

	yesFlag := update.Flags().Lookup("yes")
	if yesFlag == nil {
		t.Fatalf("subcomando update não tem a flag --yes")
	}
	if yesFlag.Shorthand != "y" {
		t.Errorf("flag --yes tem shorthand %q, esperava \"y\"", yesFlag.Shorthand)
	}

	if update.Flags().Lookup("check") == nil {
		t.Errorf("subcomando update não tem a flag --check")
	}
}

func TestRootCommandHelp(t *testing.T) {
	cmd := NewRootCommand(Version{Version: "0.1.0", Commit: "abc1234", Date: "2026-08-11T12:00:00Z"})
	cmd.SetArgs([]string{"--help"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("cmd.Execute() com --help devolveu erro inesperado: %v", err)
	}
}
