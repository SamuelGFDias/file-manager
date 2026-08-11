package splitpdf

import (
	"fmt"

	"github.com/SamuelGFDias/file-manager/internal/tool"
	"github.com/SamuelGFDias/file-manager/internal/ui"
)

// Tool implementa tool.Tool para o comando "split-pdf".
//
// Command(), params() e run() ficam em command.go; a tela interativa fica
// em screen.go; Options e seus defaults ficam em options.go.
type Tool struct {
	opts Options
}

var _ tool.Tool = (*Tool)(nil)

// New cria um novo Tool com as opções padrão.
func New() *Tool {
	return &Tool{opts: defaultOptions()}
}

// Meta devolve os metadados de identificação de split-pdf.
func (t *Tool) Meta() tool.Meta {
	return tool.Meta{
		ID:          "split-pdf",
		Title:       "Separar PDFs",
		Description: "Separa um PDF em vários arquivos por página, por intervalos de páginas ou por expressão regular no conteúdo",
	}
}

// Screen devolve a tela interativa de split-pdf.
func (t *Tool) Screen() ui.Screen {
	return &screen{tool: t}
}

// Doc devolve a documentação exportável de split-pdf.
func (t *Tool) Doc() tool.Doc {
	return tool.Doc{
		ID:      "split-pdf",
		Title:   "Separar PDFs",
		Summary: "Separa um PDF em vários arquivos menores, por página, por intervalos de páginas ou por expressão regular no conteúdo de cada página.",
		Description: "split-pdf separa um único PDF de entrada em vários arquivos de saída, segundo três modos:\n" +
			"  - page: gera um arquivo por página do PDF de entrada.\n" +
			"  - range: gera um arquivo por intervalo informado em --ranges (ex: \"1-5,6-10,11-\"), " +
			"na notação aceita por pdfcpu.\n" +
			"  - regex: aplica --regex ao texto extraído de cada página. Cada página cujo texto CASA " +
			"com a regex INICIA um novo documento de saída; as páginas seguintes que NÃO casam continuam " +
			"fazendo parte desse mesmo documento até que a próxima página com match apareça (ou até o fim " +
			"do PDF). O nome do arquivo de saída vem do primeiro grupo de captura da regex quando ele " +
			"existe (ex: \"NF:\\s*(\\d+)\" nomeia os arquivos pelo número da nota fiscal); quando não há " +
			"captura, ou ela vem vazia, o nome é gerado a partir de --name-template. No modo regex, páginas " +
			"sem texto embutido (PDFs digitalizados) passam por OCR via Tesseract, controlado por --ocr " +
			"e --ocr-lang.",
		WhenToUse: []string{
			"quando o usuário pedir para separar, dividir ou quebrar um PDF em vários arquivos",
			"quando o usuário quiser um PDF por página",
			"quando o usuário quiser extrair intervalos específicos de páginas de um PDF em arquivos separados",
			"quando o usuário tiver um PDF com vários documentos concatenados (ex: notas fiscais digitalizadas com texto) e quiser separar um por documento, usando um marcador de texto que se repete no início de cada documento",
			"quando o PDF concatenado for digitalizado (imagem sem camada de texto) e precisar de OCR para que a regex encontre o marcador de cada documento",
		},
		Flags: tool.DocFlags(t.params()),
		Examples: []tool.ExampleDoc{
			{
				Title:   "Separar um PDF em uma página por arquivo",
				Command: "file-manager split-pdf --input relatorio.pdf --mode page",
			},
			{
				Title:   "Separar um PDF em intervalos de páginas específicos",
				Command: `file-manager split-pdf --input relatorio.pdf --mode range --ranges "1-5,6-10,11-"`,
			},
			{
				Title:   "Separar um PDF com várias notas fiscais concatenadas, uma por número de nota",
				Command: `file-manager split-pdf --input notas.pdf --mode regex --regex 'NF:\s*(\d+)' --output-dir ./notas-separadas`,
			},
			{
				Title:   "Separar um PDF digitalizado (sem texto embutido) forçando OCR em todas as páginas",
				Command: `file-manager split-pdf --input notas-escaneadas.pdf --mode regex --regex 'NF:\s*(\d+)' --ocr always --ocr-lang por`,
			},
		},
		ProfileSchema: "input: documento.pdf\n" +
			"output_dir: \"\"\n" +
			"mode: page\n" +
			"ranges: \"\"\n" +
			"regex: \"\"\n" +
			"name_template: pagina-%03d\n" +
			"overwrite: false\n" +
			"ocr: auto\n" +
			"ocr_lang: por\n",
		Notes: []string{
			"No modo regex, páginas de PDFs digitalizados (sem camada de texto embutida) passam por OCR quando o Tesseract está instalado no sistema (padrão --ocr auto: OCR só nas páginas sem texto; --ocr always força OCR em todas; --ocr never desativa). Sem o Tesseract instalado, essas páginas continuam sem texto e a regex simplesmente não encontra match nelas — a ferramenta avisa disso nos Details da execução.",
			"O OCR custa cerca de 1s por página e pode errar caracteres reconhecidos; ao usar --mode regex sobre PDFs digitalizados, prefira expressões regulares mais tolerantes a esse tipo de erro.",
			"O idioma do OCR (--ocr-lang) é \"por\" por padrão e exige o pacote de idioma correspondente instalado no Tesseract; sem ele o reconhecimento roda com baixa precisão.",
			"Nomes de arquivo derivados de um grupo de captura da regex são sanitizados: separadores de caminho, sequências \"..\", caracteres inválidos em nomes de arquivo no Windows e caracteres de controle são substituídos por \"_\".",
			"Sem --overwrite, a operação falha se algum arquivo de saída já existir, em vez de sobrescrevê-lo silenciosamente.",
		},
	}
}

// profileID identifica a struct usada como tipo dinâmico esperado por
// Profile(), para gerar mensagens de erro claras em caso de asserção de
// tipo malsucedida.
const profileTypeName = "*splitpdf.Options"

// profile implementa tool.ProfileSupport para split-pdf, reusando o mesmo
// Tool (e portanto os mesmos params()/run()) usado pelo comando cobra e
// pela tela interativa.
type profile struct {
	tool *Tool
}

// Profile devolve o suporte a perfis salvos de split-pdf.
func (t *Tool) Profile() tool.ProfileSupport {
	return &profile{tool: t}
}

// Empty devolve um ponteiro para Options com os valores padrão.
func (p *profile) Empty() any {
	o := defaultOptions()
	return &o
}

// Edit carrega current em t.opts, reusa as perguntas interativas de
// split-pdf para editá-las e devolve o resultado.
func (p *profile) Edit(nav *ui.Navigator, current any) (any, error) {
	v, ok := current.(*Options)
	if !ok {
		return nil, fmt.Errorf("perfil de split-pdf: tipo inválido %T, esperava %s", current, profileTypeName)
	}

	p.tool.opts = *v

	if err := tool.PromptAll(p.tool.params()); err != nil {
		return nil, err
	}

	return &p.tool.opts, nil
}

// Apply carrega opts em t.opts e executa split-pdf com elas.
func (p *profile) Apply(opts any) (tool.Result, error) {
	v, ok := opts.(*Options)
	if !ok {
		return tool.Result{}, fmt.Errorf("perfil de split-pdf: tipo inválido %T, esperava %s", opts, profileTypeName)
	}

	p.tool.opts = *v

	return p.tool.run()
}
