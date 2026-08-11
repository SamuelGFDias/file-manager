// Command scaffold gera os arquivos iniciais de uma nova ferramenta do CLI
// file-manager, seguindo o padrão usado por internal/tools/<pacote>/.
//
// Uso:
//
//	go run ./cmd/scaffold <nome-da-ferramenta> [--force] [--root <dir>]
//
// O registro da ferramenta em internal/app/registry.go continua sendo um
// passo manual, impresso ao final da execução.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/SamuelGFDias/file-manager/internal/scaffold"
)

func main() {
	fs := flag.NewFlagSet("scaffold", flag.ContinueOnError)
	force := fs.Bool("force", false, "sobrescreve os arquivos da ferramenta se ela já existir")
	root := fs.String("root", "", "raiz do projeto (padrão: diretório de trabalho atual)")

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "uso: go run ./cmd/scaffold <nome-da-ferramenta> [--force] [--root <dir>]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Cria os arquivos iniciais de uma nova ferramenta em internal/tools/<pacote>/,")
		fmt.Fprintln(os.Stderr, "seguindo o padrão do projeto (tool.go, command.go, screen.go, options.go e")
		fmt.Fprintln(os.Stderr, "<pacote>_test.go).")
		fmt.Fprintln(os.Stderr)
		fs.PrintDefaults()
	}

	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}

	args := fs.Args()
	if len(args) != 1 {
		fs.Usage()
		os.Exit(2)
	}

	name := args[0]

	outputRoot := *root
	if outputRoot == "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "erro: não foi possível obter o diretório de trabalho atual: %v\n", err)
			os.Exit(1)
		}
		outputRoot = wd
	}

	created, err := scaffold.Generate(scaffold.Options{
		Name:       name,
		OutputRoot: outputRoot,
		Force:      *force,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro: %v\n", err)
		os.Exit(1)
	}

	// O nome já foi validado com sucesso por Generate, então este erro nunca
	// deveria ocorrer aqui — mas tratamos mesmo assim por segurança.
	names, err := scaffold.DeriveNames(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Arquivos criados:")
	for _, f := range created {
		fmt.Printf("  %s\n", f)
	}

	fmt.Println()
	fmt.Println("=== Passo manual pendente ===")
	fmt.Println("Registre a ferramenta em internal/app/registry.go:")
	fmt.Println()
	fmt.Printf("  1. Acrescente o import:\n")
	fmt.Printf("       \"github.com/SamuelGFDias/file-manager/internal/tools/%s\"\n", names.Package)
	fmt.Printf("  2. Acrescente à lista de ferramentas registradas:\n")
	fmt.Printf("       %s.New(),\n", names.Package)
}
