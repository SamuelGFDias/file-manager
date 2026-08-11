// Package scaffold gera os arquivos iniciais de uma nova ferramenta do CLI
// file-manager, seguindo o padrão usado por internal/tools/<pacote>/
// (tool.go, command.go, screen.go, options.go e <pacote>_test.go).
//
// O objetivo é que adicionar uma ferramenta nova não exija copiar e colar
// boilerplate: DeriveNames converte um nome em kebab-case nas variações
// necessárias (pacote, tipo exportado, título humano) e Generate materializa
// os templates embutidos no diretório correspondente.
//
// Generate nunca edita internal/app/registry.go nem qualquer outro arquivo
// já existente do projeto — registrar a ferramenta ali é um passo manual de
// uma linha, deliberadamente deixado de fora: editar código Go existente por
// parsing é frágil e o ganho não compensa o risco.
package scaffold

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"unicode"
)

//go:embed templates/*.tmpl
var templatesFS embed.FS

// nameRe valida nomes de ferramenta em kebab-case: começa com letra
// minúscula, segue com letras minúsculas e dígitos, com segmentos
// adicionais separados por um único hífen (sem hífen duplicado, no início
// ou no fim).
var nameRe = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// Options controla a geração de uma nova ferramenta.
type Options struct {
	// Name é o nome da ferramenta em kebab-case, ex. "compress-pdf".
	Name string
	// OutputRoot é a raiz do projeto; os arquivos vão para
	// <OutputRoot>/internal/tools/<pkg>/.
	OutputRoot string
	// Force permite sobrescrever um diretório de ferramenta já existente.
	Force bool
}

// Names guarda as variações de nome derivadas do kebab-case de entrada.
type Names struct {
	// Kebab é o nome original em kebab-case, ex. "compress-pdf". É o ID do
	// comando.
	Kebab string
	// Package é o nome do pacote Go, ex. "compresspdf".
	Package string
	// Type é o nome do tipo exportado, ex. "CompressPdf".
	Type string
	// Title é o rótulo humano, ex. "Compress Pdf".
	Title string
}

// DeriveNames converte um nome em kebab-case nas variações necessárias para
// gerar uma ferramenta (pacote, tipo exportado, título humano). Devolve erro
// se name não casar com o formato esperado
// (^[a-z][a-z0-9]*(-[a-z0-9]+)*$): letras minúsculas e dígitos, sem
// maiúsculas, sem underscore, sem começar por dígito e sem hífen duplicado
// ou nas pontas.
func DeriveNames(name string) (Names, error) {
	if !nameRe.MatchString(name) {
		return Names{}, fmt.Errorf(
			"nome de ferramenta inválido: %q — use kebab-case: comece com uma letra minúscula e use "+
				"apenas letras minúsculas e dígitos, com segmentos separados por um único hífen "+
				"(sem hífen no início/fim nem duplicado), ex.: \"compress-pdf\"",
			name,
		)
	}

	segments := strings.Split(name, "-")

	var pkg strings.Builder
	var typ strings.Builder
	titleSegments := make([]string, 0, len(segments))

	for _, seg := range segments {
		pkg.WriteString(seg)

		capitalized := capitalizeFirst(seg)
		typ.WriteString(capitalized)
		titleSegments = append(titleSegments, capitalized)
	}

	return Names{
		Kebab:   name,
		Package: pkg.String(),
		Type:    typ.String(),
		Title:   strings.Join(titleSegments, " "),
	}, nil
}

// capitalizeFirst devolve s com o primeiro rune em maiúscula, preservando o
// restante. Segmentos puramente numéricos (sem letra inicial) voltam
// inalterados.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// genFile descreve um template embutido e o nome do arquivo gerado a partir
// dele. Quando output contém "%s", ele é substituído pelo nome do pacote da
// ferramenta (usado para nomear o arquivo de teste como <pacote>_test.go).
type genFile struct {
	template string
	output   string
}

var genFiles = []genFile{
	{template: "tool.go.tmpl", output: "tool.go"},
	{template: "command.go.tmpl", output: "command.go"},
	{template: "screen.go.tmpl", output: "screen.go"},
	{template: "options.go.tmpl", output: "options.go"},
	{template: "tool_test.go.tmpl", output: "%s_test.go"},
}

// Generate cria os arquivos da nova ferramenta descrita em opts, a partir
// dos templates embutidos, e devolve a lista de caminhos absolutos criados,
// em ordem alfabética.
func Generate(opts Options) ([]string, error) {
	names, err := DeriveNames(opts.Name)
	if err != nil {
		return nil, err
	}

	dir := filepath.Join(opts.OutputRoot, "internal", "tools", names.Package)

	if info, statErr := os.Stat(dir); statErr == nil {
		if !info.IsDir() {
			return nil, fmt.Errorf("%s já existe e não é um diretório", dir)
		}
		if !opts.Force {
			return nil, fmt.Errorf(
				"a ferramenta %q já existe em %s — use --force para sobrescrever os arquivos gerados",
				names.Kebab, dir,
			)
		}
	} else if !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("erro ao verificar diretório %s: %w", dir, statErr)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("erro ao criar diretório %s: %w", dir, err)
	}

	tmpl, err := template.ParseFS(templatesFS, "templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar templates embutidos: %w", err)
	}

	created := make([]string, 0, len(genFiles))

	for _, gf := range genFiles {
		outputName := gf.output
		if strings.Contains(outputName, "%s") {
			outputName = fmt.Sprintf(outputName, names.Package)
		}

		path := filepath.Join(dir, outputName)

		if err := renderToFile(tmpl, gf.template, path, names); err != nil {
			return nil, err
		}

		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("erro ao resolver caminho absoluto de %s: %w", path, err)
		}

		created = append(created, absPath)
	}

	sort.Strings(created)

	return created, nil
}

// renderToFile executa o template nomeado templateName em tmpl com os dados
// data e grava o resultado em path.
func renderToFile(tmpl *template.Template, templateName, path string, data any) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("erro ao criar arquivo %s: %w", path, err)
	}

	execErr := tmpl.ExecuteTemplate(f, templateName, data)
	closeErr := f.Close()

	if execErr != nil {
		return fmt.Errorf("erro ao gerar %s a partir do template %s: %w", path, templateName, execErr)
	}
	if closeErr != nil {
		return fmt.Errorf("erro ao fechar %s: %w", path, closeErr)
	}

	return nil
}
