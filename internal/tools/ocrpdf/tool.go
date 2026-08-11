package ocrpdf

import (
	"fmt"

	"github.com/SamuelGFDias/file-manager/internal/tool"
	"github.com/SamuelGFDias/file-manager/internal/ui"
)

// var _ tool.Tool garante em tempo de compilação que *Tool satisfaz o
// contrato tool.Tool.
var _ tool.Tool = (*Tool)(nil)

// Tool implementa tool.Tool (e tool.ProfileSupport) para o comando
// "ocr-pdf".
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

// Meta devolve os metadados de identificação de ocr-pdf.
func (t *Tool) Meta() tool.Meta {
	return tool.Meta{
		ID:          "ocr-pdf",
		Title:       "Tornar PDF pesquisável",
		Description: "Grava a camada de texto de OCR de volta num PDF digitalizado, tornando-o pesquisável.",
	}
}

// Screen devolve a tela interativa de ocr-pdf.
func (t *Tool) Screen() ui.Screen {
	return &Screen{tool: t}
}

// Doc devolve a documentação exportável de ocr-pdf.
func (t *Tool) Doc() tool.Doc {
	return tool.Doc{
		ID:      "ocr-pdf",
		Title:   "Tornar PDF pesquisável",
		Summary: "Transforma PDFs digitalizados (imagem) em PDFs pesquisáveis de verdade, gravando a camada de texto do OCR de volta no arquivo.",
		Description: "ocr-pdf resolve as entradas de --input (arquivos e/ou pastas, repetível, com a mesma " +
			"semântica de --max-depth já usada por merge-pdf) e, para cada PDF, extrai as imagens de cada " +
			"página e o texto já embutido (sem OCR). Um arquivo só é processado quando TODAS as suas páginas " +
			"forem 'puro scan' — exatamente uma imagem, sem nenhum texto embutido: é o único caso em que " +
			"reconstruir o PDF a partir das imagens é fiel ao conteúdo original. Qualquer página com texto " +
			"junto de imagem, com mais de uma imagem, sem imagem nenhuma, ou um arquivo que já tenha texto " +
			"embutido (não é digitalizado, não há o que reconhecer) faz o arquivo inteiro ser recusado, com " +
			"um motivo explicado — a ferramenta nunca reconstrói parcialmente um arquivo arriscando perder " +
			"conteúdo em silêncio.\n\n" +
			"Cada página elegível passa pelo Tesseract, que gera um PDF de uma página com a imagem original " +
			"e uma camada de texto invisível sobreposta (~1s por página); as páginas são então unidas, NA " +
			"ORDEM DO NÚMERO DA PÁGINA, no arquivo final gravado como " +
			"'<pasta-de-saída ou pasta do original>/<nome><--suffix>.pdf' — o original nunca é sobrescrito. " +
			"Se o destino já existir, --skip-existing pula sem erro (útil para retomar um lote interrompido) " +
			"e, sem --overwrite, o arquivo também é pulado (nunca falha o lote inteiro por causa de um " +
			"arquivo). --dry-run classifica todos os arquivos e mostra quantos seriam processados e quantos " +
			"seriam pulados (e por quê), sem gerar nada — recomendado antes de rodar de verdade sobre um " +
			"lote grande, já que o processamento é lento (~1s por página).",
		WhenToUse: []string{
			"quando o usuário quiser que um PDF escaneado fique pesquisável no Explorer do Windows ou em um leitor de PDF",
			"quando o usuário reclamar que precisa reabrir/reprocessar o mesmo PDF digitalizado toda vez para buscar texto nele",
			"quando o usuário pedir para 'colocar OCR' ou 'gravar o texto' num PDF, não só para lê-lo uma vez",
		},
		Flags: tool.DocFlags(t.params()),
		Examples: []tool.ExampleDoc{
			{
				Title:   "Simular sobre uma pasta antes de processar de verdade",
				Command: "file-manager ocr-pdf --input ./digitalizados --dry-run",
			},
			{
				Title:   "Tornar pesquisáveis todos os PDFs de uma pasta, gravando ao lado de cada original",
				Command: "file-manager ocr-pdf --input ./digitalizados",
			},
			{
				Title:   "Gravar os resultados em outra pasta, com sufixo e idioma customizados",
				Command: "file-manager ocr-pdf -i ./digitalizados -o ./pesquisaveis --suffix _pesquisavel --lang eng",
			},
			{
				Title:   "Retomar um lote grande interrompido, pulando o que já foi processado",
				Command: "file-manager ocr-pdf --input ./digitalizados --skip-existing --report ./relatorio-ocr.csv",
			},
		},
		ProfileSchema: `inputs:
  - ./digitalizados
max_depth: 1
output_dir: ''
suffix: -ocr
lang: por
overwrite: false
skip_existing: false
report: ''`,
		Notes: []string{
			"Exige o Tesseract instalado no sistema — ao contrário do OCR usado como fallback de leitura em organize-pdf/split-pdf (que degrada normalmente sem ele), ocr-pdf falha com um erro claro ANTES de processar qualquer coisa, inclusive em --dry-run.",
			"Só processa arquivos cujas páginas sejam TODAS 'puro scan' (uma única imagem, sem texto embutido). Qualquer página mista (imagem + texto, ou mais de uma imagem) faz o arquivo inteiro ser recusado, com o motivo explicado: reconstruir a página a partir só da imagem perderia o conteúdo que não é imagem.",
			"Um arquivo que já tem texto embutido em todas as páginas também é recusado — por economia, não por erro: não há nada para o OCR reconhecer.",
			"Custo medido: ~0,9s por página, e o arquivo gerado costuma ficar maior que o original (o Tesseract reescreve a imagem ao montar o PDF pesquisável) — um lote de 200 documentos de 3 páginas leva quase 10 minutos, por isso o progresso é exibido arquivo a arquivo.",
			"O original NUNCA é sobrescrito: a saída é sempre um arquivo novo, com --suffix (default '-ocr'), em --output-dir (ou ao lado do original quando vazio).",
			"--dry-run classifica tudo e não gera nenhum arquivo — é o jeito de saber, antes de comprometer minutos de processamento, quantos arquivos serão processados e quantos serão pulados (e por quê).",
			"--report grava um relatório CSV desta execução (arquivo, origem, destino, processado, páginas, motivo), com BOM UTF-8 para abrir corretamente no Excel em português — mesmo formato usado por organize-pdf --report. Funciona também com --dry-run.",
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

// Edit reusa os prompts interativos de ocr-pdf para editar as Options de
// um perfil existente, devolvendo a versão editada.
func (t *Tool) Edit(nav *ui.Navigator, current any) (any, error) {
	opts, ok := current.(*Options)
	if !ok {
		return nil, fmt.Errorf("ocr-pdf: perfil com tipo inválido: esperava *ocrpdf.Options, recebeu %T", current)
	}

	t.opts = *opts
	if err := tool.PromptAll(t.params()); err != nil {
		return nil, err
	}

	return &t.opts, nil
}

// Apply executa ocr-pdf com as Options de um perfil salvo.
func (t *Tool) Apply(opts any) (tool.Result, error) {
	v, ok := opts.(*Options)
	if !ok {
		return tool.Result{}, fmt.Errorf("ocr-pdf: perfil com tipo inválido: esperava *ocrpdf.Options, recebeu %T", opts)
	}

	t.opts = *v
	return t.run()
}
