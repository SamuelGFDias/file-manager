package docs

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamuelGFDias/file-manager/internal/tool"
	"github.com/SamuelGFDias/file-manager/internal/ui"
	"github.com/spf13/cobra"
)

// fakeTool é uma implementação falsa de tool.Tool, usada apenas para os
// testes deste pacote: devolve uma Doc fixa e satisfaz o restante da
// interface com valores mínimos.
type fakeTool struct {
	doc tool.Doc
}

func (f *fakeTool) Meta() tool.Meta {
	return tool.Meta{ID: f.doc.ID, Title: f.doc.Title, Description: f.doc.Summary}
}

func (f *fakeTool) Command() *cobra.Command {
	return &cobra.Command{}
}

func (f *fakeTool) Screen() ui.Screen {
	return nil
}

func (f *fakeTool) Doc() tool.Doc {
	return f.doc
}

func (f *fakeTool) Profile() tool.ProfileSupport {
	return nil
}

func fakeTools() []tool.Tool {
	toolA := &fakeTool{doc: tool.Doc{
		ID:          "merge-pdf",
		Title:       "Unir PDFs",
		Summary:     "Une múltiplos arquivos PDF em um único arquivo.",
		Description: "Recebe uma lista de arquivos PDF e os concatena, na ordem informada, em um único arquivo de saída.",
		WhenToUse: []string{
			"quando o usuário pedir para juntar/unir vários PDFs em um só",
		},
		Flags: []tool.FlagDoc{
			{Name: "output", Shorthand: "o", Type: "string", Default: "output.pdf", Description: "Caminho do arquivo de saída.", Example: "merged.pdf"},
			{Name: "input", Shorthand: "i", Type: "stringSlice", Default: "", Description: "Arquivos PDF de entrada.", Example: "a.pdf,b.pdf"},
		},
		Examples: []tool.ExampleDoc{
			{Title: "Unir dois arquivos", Command: "file-manager merge-pdf --input a.pdf,b.pdf --output merged.pdf"},
		},
		ProfileSchema: "id: merge-pdf\noutput: output.pdf\n",
		Notes: []string{
			"Os arquivos são unidos na ordem em que aparecem na flag --input.",
		},
	}}

	toolB := &fakeTool{doc: tool.Doc{
		ID:          "split-pdf",
		Title:       "Separar PDFs",
		Summary:     "Separa um PDF em múltiplos arquivos.",
		Description: "Recebe um arquivo PDF e o divide em vários arquivos, uma página (ou intervalo) por arquivo de saída.",
		WhenToUse: []string{
			"quando o usuário pedir para separar/dividir um PDF em partes",
		},
		Flags: []tool.FlagDoc{
			{Name: "input", Shorthand: "i", Type: "string", Default: "", Description: "Arquivo PDF de entrada.", Example: "doc.pdf"},
		},
		Examples: []tool.ExampleDoc{
			{Title: "Separar por página", Command: "file-manager split-pdf --input doc.pdf --pages 1"},
		},
		ProfileSchema: "", // sem suporte a perfil
		Notes:         nil,
	}}

	return []tool.Tool{toolA, toolB}
}

// fakeCommands devolve uma lista mínima de tool.Doc representando comandos
// auxiliares (não-ferramentas), no mesmo espírito de fakeTools acima —
// usada para testar que Render/Export incluem .Commands sem depender de
// internal/commanddocs (que importaria este pacote de volta, criando um
// ciclo: internal/commanddocs não importa internal/ui/docs, mas testar com
// dados falsos evita qualquer acoplamento entre os dois pacotes de teste).
func fakeCommands() []tool.Doc {
	return []tool.Doc{
		{
			ID:      "undo",
			Title:   "Desfazer (undo)",
			Summary: "Desfaz uma operação de organização registrada.",
			Description: "Reverte uma operação registrada no histórico: apaga o que foi criado " +
				"(cópia) ou devolve à origem (mover).",
			WhenToUse: []string{"quando o usuário pedir para desfazer uma organização"},
			Flags: []tool.FlagDoc{
				{Name: "id", Type: "string", Description: "ID da operação a desfazer."},
				{Name: "last", Type: "bool", Description: "Desfaz a operação mais recente."},
			},
			Examples: []tool.ExampleDoc{
				{Title: "Desfazer a última operação", Command: "file-manager undo --last"},
			},
		},
		{
			ID:          "version",
			Title:       "Versão (version)",
			Summary:     "Mostra a versão do binário.",
			Description: "Imprime a versão, commit e data de build.",
			Examples: []tool.ExampleDoc{
				{Title: "Mostrar a versão", Command: "file-manager version"},
			},
		},
	}
}

func TestParseFormat(t *testing.T) {
	tests := []struct {
		in      string
		want    Format
		wantErr bool
	}{
		{"context", FormatContext, false},
		{"skill", FormatSkill, false},
		{"", FormatContext, false},
		{"xml", "", true},
	}

	for _, tt := range tests {
		got, err := ParseFormat(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseFormat(%q): esperava erro, obteve nil", tt.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseFormat(%q): erro inesperado: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseFormat(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRenderContext(t *testing.T) {
	tools := fakeTools()
	commands := fakeCommands()

	out, err := Render(FormatContext, tools, commands, "1.2.3")
	if err != nil {
		t.Fatalf("Render(FormatContext) erro: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("Render(FormatContext) devolveu conteúdo vazio")
	}

	content := string(out)

	for _, tl := range tools {
		d := tl.Doc()
		if !strings.Contains(content, d.Title) {
			t.Errorf("conteúdo não contém o título da ferramenta %q", d.Title)
		}
		for _, f := range d.Flags {
			if !strings.Contains(content, f.Name) {
				t.Errorf("conteúdo não contém a flag %q", f.Name)
			}
		}
		for _, ex := range d.Examples {
			if !strings.Contains(content, ex.Command) {
				t.Errorf("conteúdo não contém o comando de exemplo %q", ex.Command)
			}
		}
	}
}

// TestRenderContextIncludesCommands prova que Render(FormatContext) também
// cobre os comandos auxiliares (undo, profiles, update, version, docs
// export — ver internal/commanddocs), não só as ferramentas de app.Tools().
// É o teste que teria pegado a lacuna original: antes desta mudança,
// Render só recebia []tool.Tool e nunca tinha como imprimir nada além do
// que vinha do registry.
func TestRenderContextIncludesCommands(t *testing.T) {
	commands := fakeCommands()

	out, err := Render(FormatContext, fakeTools(), commands, "1.2.3")
	if err != nil {
		t.Fatalf("Render(FormatContext) erro: %v", err)
	}
	content := string(out)

	if !strings.Contains(content, "Comandos auxiliares") {
		t.Error("conteúdo não contém a seção \"Comandos auxiliares\"")
	}

	for _, d := range commands {
		if !strings.Contains(content, d.Title) {
			t.Errorf("conteúdo não contém o título do comando auxiliar %q", d.Title)
		}
		for _, f := range d.Flags {
			if !strings.Contains(content, "--"+f.Name) {
				t.Errorf("conteúdo não contém a flag %q do comando auxiliar %q", f.Name, d.ID)
			}
		}
		for _, ex := range d.Examples {
			if !strings.Contains(content, ex.Command) {
				t.Errorf("conteúdo não contém o comando de exemplo %q do comando auxiliar %q", ex.Command, d.ID)
			}
		}
	}
}

func TestRenderContext_EmptyProfileSchemaDoesNotLeakEmptyYamlBlock(t *testing.T) {
	tools := fakeTools()

	out, err := Render(FormatContext, tools, fakeCommands(), "1.2.3")
	if err != nil {
		t.Fatalf("Render(FormatContext) erro: %v", err)
	}
	content := string(out)

	// O schema da ferramenta que tem ProfileSchema deve aparecer.
	if !strings.Contains(content, "id: merge-pdf") {
		t.Error("conteúdo não contém o ProfileSchema da ferramenta que suporta perfil")
	}

	// Não deve existir um bloco ```yaml vazio (sem conteúdo entre a
	// abertura e o fechamento), que seria o artefato deixado pela
	// ferramenta com ProfileSchema == "".
	if strings.Contains(content, "```yaml\n```") || strings.Contains(content, "```yaml\n\n```") {
		t.Error("conteúdo contém um bloco ```yaml vazio, indicando vazamento de ProfileSchema vazio")
	}
}

func TestRenderSkill(t *testing.T) {
	tools := fakeTools()

	out, err := Render(FormatSkill, tools, fakeCommands(), "1.2.3")
	if err != nil {
		t.Fatalf("Render(FormatSkill) erro: %v", err)
	}

	content := string(out)

	if !strings.HasPrefix(content, "---\n") {
		preview := content
		if len(preview) > 20 {
			preview = preview[:20]
		}
		t.Errorf("Render(FormatSkill) não começa com frontmatter '---\\n', começa com: %q", preview)
	}

	if !strings.Contains(content, "name: file-manager") {
		t.Error("Render(FormatSkill) não contém 'name: file-manager'")
	}
}

// TestRenderSkillIncludesCommands prova que Render(FormatSkill) também
// cobre os comandos auxiliares, com o mesmo raciocínio de
// TestRenderContextIncludesCommands: sem isso, um SKILL.md instalado num
// agente de IA nunca saberia que "undo", "profiles", "update", "version" e
// "docs export" existem.
func TestRenderSkillIncludesCommands(t *testing.T) {
	commands := fakeCommands()

	out, err := Render(FormatSkill, fakeTools(), commands, "1.2.3")
	if err != nil {
		t.Fatalf("Render(FormatSkill) erro: %v", err)
	}
	content := string(out)

	if !strings.Contains(content, "Comandos auxiliares") {
		t.Error("conteúdo não contém a seção \"Comandos auxiliares\"")
	}

	for _, d := range commands {
		if !strings.Contains(content, d.Title) {
			t.Errorf("conteúdo não contém o título do comando auxiliar %q", d.Title)
		}
		for _, ex := range d.Examples {
			if !strings.Contains(content, ex.Command) {
				t.Errorf("conteúdo não contém o comando de exemplo %q do comando auxiliar %q", ex.Command, d.ID)
			}
		}
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	tools := fakeTools()
	commands := fakeCommands()

	out1, err := Render(FormatContext, tools, commands, "1.2.3")
	if err != nil {
		t.Fatalf("Render erro: %v", err)
	}
	out2, err := Render(FormatContext, tools, commands, "1.2.3")
	if err != nil {
		t.Fatalf("Render erro: %v", err)
	}

	if !bytes.Equal(out1, out2) {
		t.Error("Render(FormatContext) não é determinístico entre chamadas")
	}

	skill1, err := Render(FormatSkill, tools, commands, "1.2.3")
	if err != nil {
		t.Fatalf("Render erro: %v", err)
	}
	skill2, err := Render(FormatSkill, tools, commands, "1.2.3")
	if err != nil {
		t.Fatalf("Render erro: %v", err)
	}

	if !bytes.Equal(skill1, skill2) {
		t.Error("Render(FormatSkill) não é determinístico entre chamadas")
	}
}

func TestExport(t *testing.T) {
	tools := fakeTools()
	commands := fakeCommands()

	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "que", "nao", "existe", "docs.md")

	if err := Export(FormatContext, path, tools, commands, "1.2.3"); err != nil {
		t.Fatalf("Export erro: %v", err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("erro ao ler arquivo exportado: %v", err)
	}

	rendered, err := Render(FormatContext, tools, commands, "1.2.3")
	if err != nil {
		t.Fatalf("Render erro: %v", err)
	}

	if !bytes.Equal(written, rendered) {
		t.Error("conteúdo gravado por Export difere do conteúdo de Render")
	}
}
