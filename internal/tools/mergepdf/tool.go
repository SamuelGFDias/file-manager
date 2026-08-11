package mergepdf

import (
	"fmt"

	"github.com/SamuelGFDias/file-manager/internal/tool"
	"github.com/SamuelGFDias/file-manager/internal/ui"
)

// var _ tool.Tool garante em tempo de compilação que *Tool satisfaz o
// contrato tool.Tool.
var _ tool.Tool = (*Tool)(nil)

// Tool implementa tool.Tool (e tool.ProfileSupport) para o comando
// "merge-pdf".
//
// Command(), params() e run() ficam em command.go; a tela interativa fica
// em screen.go; Options e seus defaults ficam em options.go.
type Tool struct {
	opts Options
}

// New cria um novo Tool com as opções padrão.
func New() *Tool {
	return &Tool{opts: defaultOptions()}
}

// Meta devolve os metadados de identificação de merge-pdf.
func (t *Tool) Meta() tool.Meta {
	return tool.Meta{
		ID:          "merge-pdf",
		Title:       "Unir PDFs",
		Description: "Une vários arquivos PDF (ou os PDFs de uma ou mais pastas) em um único arquivo.",
	}
}

// Screen devolve a tela interativa de merge-pdf.
func (t *Tool) Screen() ui.Screen {
	return &Screen{tool: t}
}

// Doc devolve a documentação exportável de merge-pdf.
func (t *Tool) Doc() tool.Doc {
	return tool.Doc{
		ID:      "merge-pdf",
		Title:   "Unir PDFs",
		Summary: "Une múltiplos PDFs (arquivos e/ou pastas) em um único arquivo de saída.",
		Description: "Recebe uma lista de entradas (--input, repetível), cada uma um arquivo PDF ou " +
			"uma pasta. Pastas são varridas em busca de arquivos .pdf conforme --max-depth: 0 inclui " +
			"apenas os PDFs diretamente dentro da pasta informada, N>0 desce até N níveis de " +
			"subpastas, e -1 varre todos os níveis sem limite. Os PDFs resolvidos são então unidos, " +
			"na ordem definida por --sort, em um único arquivo gravado em --output.",
		WhenToUse: []string{
			"quando o usuário pedir para juntar, unir ou combinar vários PDFs num só",
			"quando o usuário quiser consolidar todos os PDFs de uma pasta (e, opcionalmente, de suas subpastas) em um único arquivo",
		},
		Flags: tool.DocFlags(t.params()),
		Examples: []tool.ExampleDoc{
			{
				Title:   "Unir dois arquivos específicos",
				Command: "file-manager merge-pdf -i a.pdf -i b.pdf -o unido.pdf",
			},
			{
				Title:   "Unir os PDFs de uma pasta e suas subpastas diretas",
				Command: "file-manager merge-pdf -i ./contratos -o ./contratos/unido.pdf",
			},
			{
				Title:   "Unir todos os PDFs recursivamente, sem limite de profundidade",
				Command: "file-manager merge-pdf -i ./relatorios --max-depth -1 -o ./relatorios/unido.pdf",
			},
			{
				Title:   "Sobrescrever o arquivo de saída caso já exista",
				Command: "file-manager merge-pdf -i a.pdf -i b.pdf -o unido.pdf --overwrite",
			},
		},
		ProfileSchema: `inputs:
  - ./contratos
  - ./anexos/extra.pdf
max_depth: 1
output: ./unido.pdf
sort: name
overwrite: false`,
		Notes: []string{
			"A ordem dos arquivos na união segue --sort: \"name\" ordena por nome, \"mtime\" por data de modificação.",
			"Pastas informadas em --input são varridas conforme --max-depth: 0 só inclui PDFs diretamente na pasta, N desce N níveis, -1 varre sem limite.",
			"Por padrão a ferramenta não sobrescreve um arquivo de saída já existente; use --overwrite para permitir.",
		},
	}
}

// Profile devolve o próprio Tool como suporte a perfis salvos: *Tool
// implementa tool.ProfileSupport via Empty/Edit/Apply, abaixo.
func (t *Tool) Profile() tool.ProfileSupport {
	return t
}

// Empty devolve um ponteiro para Options zerada com os defaults aplicados,
// usada como ponto de partida ao criar um novo perfil salvo.
func (t *Tool) Empty() any {
	opts := defaultOptions()
	return &opts
}

// Edit reusa os prompts interativos de merge-pdf para editar as Options de
// um perfil existente, devolvendo a versão editada.
func (t *Tool) Edit(nav *ui.Navigator, current any) (any, error) {
	opts, ok := current.(*Options)
	if !ok {
		return nil, fmt.Errorf("merge-pdf: perfil com tipo inválido: esperava *mergepdf.Options, recebeu %T", current)
	}

	t.opts = *opts
	if err := tool.PromptAll(t.params()); err != nil {
		return nil, err
	}

	return &t.opts, nil
}

// Apply executa merge-pdf com as Options de um perfil salvo.
func (t *Tool) Apply(opts any) (tool.Result, error) {
	v, ok := opts.(*Options)
	if !ok {
		return tool.Result{}, fmt.Errorf("merge-pdf: perfil com tipo inválido: esperava *mergepdf.Options, recebeu %T", opts)
	}

	t.opts = *v
	return t.run()
}
