// Package ocrpdf implementa a ferramenta "ocr-pdf": transforma PDFs
// digitalizados (imagem, sem camada de texto) em PDFs pesquisáveis de
// verdade, gravando a camada de texto reconhecida por OCR de volta no
// arquivo — ao contrário do OCR usado como fallback de leitura em
// organize-pdf/split-pdf, que só usa o texto reconhecido em memória, sem
// alterar o arquivo original.
//
// A regra central (ver internal/pdfutil/ocrize.go e AGENTS.md) é
// conservadora de propósito: a ferramenta reconstrói o PDF de saída a
// partir das imagens extraídas de cada página, o que é fiel quando a
// página é puro scan (uma única imagem, sem texto) e destrutivo quando não
// é (imagem + texto, ou várias imagens). Por isso só processa arquivos
// cujas páginas sejam TODAS puro scan, recusando os demais com um motivo
// claro em vez de arriscar perder conteúdo em silêncio.
package ocrpdf

// Options descreve a configuração de uma execução de ocr-pdf. É o que é
// vinculado às flags do cobra, perguntado nos prompts interativos e, via
// Profile, persistido como perfil salvo.
type Options struct {
	// Inputs são os arquivos PDF e/ou pastas a considerar. Pastas são
	// varridas conforme MaxDepth — mesma semântica de merge-pdf.
	Inputs []string `yaml:"inputs"`
	// MaxDepth controla a profundidade de varredura de pastas em Inputs:
	// 0 = só a pasta informada; N = desce N níveis; -1 = ilimitado.
	MaxDepth int `yaml:"max_depth"`
	// OutputDir é a pasta de saída dos PDFs pesquisáveis gerados; vazio
	// (default) grava cada um ao lado do original correspondente.
	OutputDir string `yaml:"output_dir"`
	// Suffix é o sufixo acrescentado ao nome do arquivo gerado — o
	// original nunca é sobrescrito.
	Suffix string `yaml:"suffix"`
	// Lang é o idioma usado pelo OCR (ex: "por", "eng").
	Lang string `yaml:"lang"`
	// Overwrite permite sobrescrever um arquivo de saída já existente.
	Overwrite bool `yaml:"overwrite"`
	// SkipExisting pula (sem erro) um arquivo cuja saída já existe —
	// pensado para retomar um lote grande interrompido no meio, já que o
	// processamento leva ~1s por página.
	SkipExisting bool `yaml:"skip_existing"`
	// DryRun só classifica os arquivos e mostra o que seria feito, sem
	// gerar nada. Nunca é persistido no perfil: é sempre decidido na hora
	// da execução (mesma convenção de organize-pdf).
	DryRun bool `yaml:"-"`
	// Report é o caminho de um relatório CSV desta execução (uma linha
	// por arquivo considerado, processado ou não, com o motivo quando
	// não processado); vazio (default) = não gera. Ao contrário de
	// DryRun, É persistido no perfil — é razoável querer sempre gerar o
	// relatório no mesmo caminho toda vez que um perfil é aplicado.
	Report string `yaml:"report"`
}

// defaultOptions devolve as Options padrão de ocr-pdf.
func defaultOptions() Options {
	return Options{
		MaxDepth: 1,
		Suffix:   "-ocr",
		Lang:     "por",
	}
}
