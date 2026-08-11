package organizepdf

import (
	"fmt"

	"github.com/SamuelGFDias/file-manager/internal/tool"
	"github.com/SamuelGFDias/file-manager/internal/ui"
)

// Tool implementa tool.Tool (e tool.ProfileSupport) para o comando
// "organize-pdf".
//
// Command(), params(), runWith() e run() ficam em command.go; o fluxo
// interativo (incluindo a calibração por exemplo e o ciclo de teste antes
// de aplicar) fica em screen.go; Options, LevelSpec e as funções puras
// ParseLevelFlags/BuildLevels ficam em options.go.
type Tool struct {
	opts Options
	// rawLevels acumula os valores crus da flag repetível --level (formato
	// "rótulo=regex"), vinda do cobra. É convertida para []LevelSpec via
	// ParseLevelFlags dentro de runWith(). O modo interativo não usa este
	// campo: ele monta t.opts.Levels diretamente, um LevelSpec por vez, à
	// medida que calibra cada nível.
	rawLevels []string
}

var _ tool.Tool = (*Tool)(nil)

// New cria um novo Tool com as opções padrão.
func New() *Tool {
	return &Tool{opts: defaultOptions()}
}

// Meta devolve os metadados de identificação de organize-pdf.
func (t *Tool) Meta() tool.Meta {
	return tool.Meta{
		ID:          "organize-pdf",
		Title:       "Organizar PDFs",
		Description: "Classifica PDFs numa hierarquia de pastas de destino a partir do texto de cada arquivo, e renomeia cada um pelo valor capturado.",
	}
}

// Screen devolve a tela interativa de organize-pdf.
func (t *Tool) Screen() ui.Screen {
	return &screen{tool: t}
}

// Doc devolve a documentação exportável de organize-pdf.
func (t *Tool) Doc() tool.Doc {
	return tool.Doc{
		ID:      "organize-pdf",
		Title:   "Organizar PDFs",
		Summary: "Classifica PDFs inteiros de uma pasta numa hierarquia de pastas de destino, com base em valores encontrados no texto de cada arquivo, e renomeia cada um pelo valor capturado.",
		Description: "organize-pdf lê cada PDF de --input, extrai seu texto e aplica, em ordem, cada nível " +
			"declarado em --level (repetível): a ORDEM em que --level aparece na linha de comando é a " +
			"ordem da hierarquia de pastas no destino (o primeiro --level vira o primeiro nível de pasta, " +
			"e assim por diante). Cada nível é um par \"rótulo=regex\"; o grupo de captura da regex (ou o " +
			"trecho inteiro casado, se não houver grupo) vira o nome do componente de pasta correspondente. " +
			"Sem nenhum --level, a ferramenta funciona em modo \"somente renomear\": os arquivos vão " +
			"diretamente para --output, sem subpastas — útil quando só se quer dar nomes melhores aos " +
			"arquivos, sem reorganizá-los. --filename-regex, quando informado, funciona do mesmo jeito para " +
			"nomear o próprio arquivo; vazio mantém o nome original.\n\n" +
			"O padrão é COPIAR os arquivos (--move não informado): os originais em --input nunca são " +
			"tocados. Use --move para mover de fato, esvaziando a pasta de origem à medida que organiza.\n\n" +
			"PDFs digitalizados (sem camada de texto) passam por OCR via Tesseract quando --ocr é \"auto\" " +
			"(padrão, só nas páginas sem texto) ou \"always\" (força OCR em todas as páginas). --ocr never " +
			"desliga o OCR por completo. Sem o Tesseract instalado, o comportamento é o mesmo de --ocr " +
			"never: esses arquivos continuam sem texto e caem em --unclassified-dir.\n\n" +
			"Um arquivo cujo texto não casa com algum nível (ou com --filename-regex) NUNCA é perdido nem " +
			"interrompe o lote: ele é copiado/movido para a subpasta --unclassified-dir dentro de --output, " +
			"e o motivo exato da falha (qual nível, ou o nome do arquivo) é reportado no resultado.",
		WhenToUse: []string{
			"quando o usuário tiver uma pasta cheia de PDFs (ex: notas fiscais, contratos, boletos) e quiser organizá-los automaticamente em subpastas a partir de algo escrito no próprio documento",
			"quando o usuário quiser renomear em lote vários PDFs pelo conteúdo (ex: pelo número da nota fiscal), sem necessariamente movê-los para subpastas",
			"quando o usuário mencionar classificar, separar por fornecedor/filial/cliente, ou organizar PDFs \"pelo conteúdo\" ou \"pelo texto\"",
		},
		Flags: tool.DocFlags(t.params()),
		Examples: []tool.ExampleDoc{
			{
				Title: "Organizar notas fiscais por fornecedor e filial, renomeando pelo número da nota",
				Command: `file-manager organize-pdf --input ./notas --output ./organizado ` +
					`--level "fornecedor=FORNECEDOR:\s*(\w+)" --level "filial=FILIAL\s*(\d+)" ` +
					`--filename-regex "NF:\s*(\d+)"`,
			},
			{
				Title:   "Somente renomear os arquivos pelo número da nota, sem criar subpastas (nenhum --level)",
				Command: `file-manager organize-pdf --input ./notas --output ./renomeado --filename-regex "NF:\s*(\d+)"`,
			},
			{
				Title: "Simular a organização antes de aplicar, sem alterar nada",
				Command: `file-manager organize-pdf --input ./notas --output ./organizado ` +
					`--level "fornecedor=FORNECEDOR:\s*(\w+)" --dry-run`,
			},
			{
				Title: "Organizar movendo os arquivos (em vez de copiar), sobrescrevendo destinos existentes",
				Command: `file-manager organize-pdf --input ./notas --output ./organizado ` +
					`--level "fornecedor=FORNECEDOR:\s*(\w+)" --move --overwrite`,
			},
			{
				Title: "Organizar uma pasta com notas fiscais digitalizadas (sem camada de texto), forçando OCR em inglês",
				Command: `file-manager organize-pdf --input ./notas-escaneadas --output ./organizado ` +
					`--level "fornecedor=SUPPLIER:\s*(\w+)" --ocr always --ocr-lang eng`,
			},
		},
		ProfileSchema: `input_dir: ./notas
output_dir: ./organizado
levels:
  - label: fornecedor
    regex: 'FORNECEDOR:\s*(\w+)'
  - label: filial
    regex: 'FILIAL\s*(\d+)'
filename_regex: 'NF:\s*(\d+)'
move: false
unclassified_dir: sem-classificacao
overwrite: false
ocr: auto
ocr_lang: por`,
		Notes: []string{
			"PDFs digitalizados (imagem sem camada de texto) passam por OCR via Tesseract quando --ocr é \"auto\" (padrão, aplica OCR só nas páginas sem texto embutido) ou \"always\" (força OCR em todas as páginas, ignorando texto embutido); --ocr never desliga o OCR. Sem o Tesseract instalado no sistema, esses PDFs continuam sem texto para casar com qualquer regex e caem em --unclassified-dir, mesmo com --ocr auto ou always.",
			"OCR é sensivelmente mais lento que a extração de texto embutido (da ordem de ~1s por página) e pode errar caracteres (ex: confundir \"0\" com \"O\"), então regex calibradas contra texto de OCR costumam se beneficiar de padrões mais tolerantes do que as calibradas contra texto embutido limpo.",
			"O idioma padrão do OCR é \"por\" (--ocr-lang); ele exige o pacote de idioma português do Tesseract instalado — sem ele, o reconhecimento cai no idioma padrão do Tesseract e a precisão tende a cair bastante.",
			"Recomenda-se sempre testar com --dry-run antes de aplicar de verdade, especialmente ao calibrar uma regex nova — o modo interativo já inclui esse teste como etapa obrigatória antes de aplicar.",
			"Nomes de pasta e de arquivo derivados de um grupo de captura são sanitizados: separadores de caminho, sequências \"..\", caracteres inválidos em nomes de arquivo no Windows e caracteres de controle são substituídos por \"_\".",
			"dry_run e sample nunca são persistidos num perfil salvo: são sempre decididos na hora da execução, nunca fazem parte da configuração salva.",
		},
	}
}

// profileTypeName identifica o tipo dinâmico esperado por Profile(), usado
// para gerar mensagens de erro claras em caso de asserção de tipo
// malsucedida.
const profileTypeName = "*organizepdf.Options"

// profile implementa tool.ProfileSupport para organize-pdf, reusando o
// mesmo Tool (e portanto os mesmos params()/run()/configure()) usado pelo
// comando cobra e pela tela interativa.
type profile struct {
	tool *Tool
}

// Profile devolve o suporte a perfis salvos de organize-pdf.
func (t *Tool) Profile() tool.ProfileSupport {
	return &profile{tool: t}
}

// Empty devolve um ponteiro para Options com os valores padrão.
func (p *profile) Empty() any {
	o := defaultOptions()
	return &o
}

// Edit carrega current em t.opts e refaz o fluxo de configuração usado pela
// tela interativa (escolha de pastas, calibração dos níveis, calibração do
// nome de arquivo, copiar ou mover), devolvendo o resultado. Não inclui o
// ciclo de teste/aplicação da tela: Edit apenas coleta e devolve as
// Options, sem executar nada.
func (p *profile) Edit(nav *ui.Navigator, current any) (any, error) {
	v, ok := current.(*Options)
	if !ok {
		return nil, fmt.Errorf("perfil de organize-pdf: tipo inválido %T, esperava %s", current, profileTypeName)
	}

	p.tool.opts = *v
	p.tool.rawLevels = nil

	if _, err := p.tool.configure(); err != nil {
		return nil, err
	}

	return &p.tool.opts, nil
}

// Apply carrega opts em t.opts e executa organize-pdf com elas.
func (p *profile) Apply(opts any) (tool.Result, error) {
	v, ok := opts.(*Options)
	if !ok {
		return tool.Result{}, fmt.Errorf("perfil de organize-pdf: tipo inválido %T, esperava %s", opts, profileTypeName)
	}

	p.tool.opts = *v
	p.tool.rawLevels = nil

	return p.tool.run()
}
