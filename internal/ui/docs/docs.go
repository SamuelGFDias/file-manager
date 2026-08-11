// Package docs gera documentação exportável do CLI file-manager, em dois
// formatos Markdown, a partir da mesma fonte de dados usada pelo binário
// real (tool.Doc de cada ferramenta), para que a documentação nunca divirja
// do comportamento efetivamente implementado.
package docs

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/SamuelGFDias/file-manager/internal/tool"
)

//go:embed context.md.tmpl skill.md.tmpl
var templatesFS embed.FS

// Format identifica o formato de documentação exportável.
type Format string

const (
	// FormatContext é o documento rico e exaustivo para colar numa
	// conversa pontual com uma IA.
	FormatContext Format = "context"
	// FormatSkill é o arquivo no formato de skill de agente de IA
	// (frontmatter YAML + corpo Markdown), para instalação persistente.
	FormatSkill Format = "skill"
)

// ParseFormat valida e converte a string vinda da flag --format. String
// vazia devolve FormatContext (o default). Qualquer outro valor que não
// "context" nem "skill" devolve erro.
func ParseFormat(s string) (Format, error) {
	switch s {
	case "":
		return FormatContext, nil
	case string(FormatContext):
		return FormatContext, nil
	case string(FormatSkill):
		return FormatSkill, nil
	default:
		return "", fmt.Errorf(
			"formato de documentação inválido: %q (valores válidos: %q, %q)",
			s, FormatContext, FormatSkill,
		)
	}
}

// templateData é a estrutura de dados exposta aos templates.
type templateData struct {
	Version string
	Tools   []tool.Doc
	// Commands é a documentação dos comandos auxiliares do CLI que não são
	// ferramentas do registry (undo, profiles, update, version, docs
	// export) — ver internal/commanddocs. Sem esta lista, docs.Render
	// nunca teria como incluí-los: ela só percorre app.Tools().
	Commands []tool.Doc
}

// templateName devolve o nome do arquivo de template embutido
// correspondente a format.
func templateName(format Format) (string, error) {
	switch format {
	case FormatContext:
		return "context.md.tmpl", nil
	case FormatSkill:
		return "skill.md.tmpl", nil
	default:
		return "", fmt.Errorf(
			"formato de documentação inválido: %q (valores válidos: %q, %q)",
			format, FormatContext, FormatSkill,
		)
	}
}

// Render gera o Markdown do formato pedido a partir das Docs das
// ferramentas e dos comandos auxiliares informados. É determinístico: a
// mesma entrada sempre produz exatamente os mesmos bytes.
//
// commands é a documentação dos comandos do CLI que não são ferramentas do
// registry (undo, profiles, update, version, docs export — ver
// internal/commanddocs.CommandDocs()). Sem este parâmetro, a documentação
// exportada só cobriria o que está em app.Tools(), que é exatamente a
// lacuna que este parâmetro existe para fechar.
func Render(format Format, tools []tool.Tool, commands []tool.Doc, version string) ([]byte, error) {
	name, err := templateName(format)
	if err != nil {
		return nil, err
	}

	tmpl, err := template.New(name).ParseFS(templatesFS, name)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar template de documentação %q: %w", name, err)
	}

	docsList := make([]tool.Doc, 0, len(tools))
	for _, t := range tools {
		docsList = append(docsList, t.Doc())
	}

	data := templateData{
		Version:  version,
		Tools:    docsList,
		Commands: commands,
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return nil, fmt.Errorf("erro ao renderizar documentação: %w", err)
	}

	return buf.Bytes(), nil
}

// Export escreve o resultado de Render em path, criando os diretórios
// necessários.
func Export(format Format, path string, tools []tool.Tool, commands []tool.Doc, version string) error {
	content, err := Render(format, tools, commands, version)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("erro ao criar diretório %q: %w", dir, err)
	}

	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("erro ao gravar arquivo %q: %w", path, err)
	}

	return nil
}
