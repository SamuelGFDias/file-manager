package pdfutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// SearchablePDFEngine é o motor capaz de gerar, a partir de uma imagem, um
// PDF de uma página com a imagem original e uma camada de texto invisível
// sobreposta. A interface é declarada aqui (e não importada do pacote
// internal/ocr) pelo mesmo motivo de OCREngine em textextract.go: manter
// pdfutil testável com um motor falso, sem depender de um processo externo
// (tesseract) nos testes deste pacote.
type SearchablePDFEngine interface {
	Available() bool
	ImageToSearchablePDF(ctx context.Context, imagePath, outBase, lang string) error
}

// PageKind classifica uma página de um PDF para decidir se ocr-pdf pode
// reconstruí-la com segurança a partir das imagens extraídas.
type PageKind int

const (
	// PagePureScan é uma página com exatamente uma imagem e sem texto
	// embutido — o caso em que reconstruir o PDF a partir da imagem (a
	// abordagem inteira de ocr-pdf) é fiel ao conteúdo original.
	PagePureScan PageKind = iota
	// PageHasText já tem texto embutido: não precisa de OCR, e portanto
	// não é o alvo desta ferramenta (ver DecideEligibility).
	PageHasText
	// PageMixed tem imagem E texto, ou mais de uma imagem. Reconstruir a
	// página só a partir da(s) imagem(ns) perderia o restante do
	// conteúdo (texto nativo, ou as demais imagens) — por isso nunca é
	// elegível para ocr-pdf.
	PageMixed
	// PageNoImage não tem imagem nem texto — não há nada para o OCR ler
	// nem texto para preservar, mas também não é um "puro scan" válido.
	PageNoImage
)

// PageInfo descreve uma única página, já classificada, de um arquivo
// considerado por ocr-pdf.
type PageInfo struct {
	// Number é o número da página, 1-based.
	Number int
	Kind   PageKind
	// ImageCount é a quantidade de imagens extraídas associadas a esta
	// página (via ParseExtractedImageName).
	ImageCount int
	// HasText indica se a página tem texto embutido (extração nativa,
	// sem OCR — ver ExtractPageTexts).
	HasText bool
}

// classifyPage decide o PageKind de uma única página a partir do texto
// embutido (já extraído, sem OCR) e da contagem de imagens associadas a
// ela. É a regra central de decisão de ocr-pdf: só uma página com
// exatamente uma imagem e nenhum texto é "puro scan" — qualquer coisa a
// mais (texto junto da imagem, ou mais de uma imagem) é PageMixed, porque
// reconstruir a página inteira a partir da(s) imagem(ns) perderia esse
// conteúdo extra.
func classifyPage(text string, imageCount int) PageKind {
	hasText := strings.TrimSpace(text) != ""

	switch {
	case imageCount == 0 && !hasText:
		return PageNoImage
	case imageCount == 1 && !hasText:
		return PagePureScan
	case imageCount == 0 && hasText:
		return PageHasText
	default:
		// imageCount >= 1 && hasText, ou imageCount >= 2 (com ou sem
		// texto): em qualquer um dos dois casos, reconstruir a página só
		// a partir da(s) imagem(ns) descartaria conteúdo real.
		return PageMixed
	}
}

// ClassifyPages classifica cada página de um arquivo a partir do texto
// embutido de cada página (pageTexts, índice 0 = página 1 — mesma convenção
// de ExtractPageTexts) e da contagem de imagens extraídas por página
// (imagesPerPage, chave = número de página 1-based, ausente = zero
// imagens). Função pura, sobre dados já coletados: não abre nenhum PDF nem
// toca em disco — quem chama (OCRize) é responsável por extrair esses dois
// insumos antes.
func ClassifyPages(pageTexts []string, imagesPerPage map[int]int) []PageInfo {
	pages := make([]PageInfo, 0, len(pageTexts))
	for i, text := range pageTexts {
		number := i + 1
		imgCount := imagesPerPage[number]
		pages = append(pages, PageInfo{
			Number:     number,
			Kind:       classifyPage(text, imgCount),
			ImageCount: imgCount,
			HasText:    strings.TrimSpace(text) != "",
		})
	}
	return pages
}

// FileEligibility é a decisão sobre um arquivo inteiro: pode (Eligible)
// ser processado por ocr-pdf, ou não — caso em que Reason explica o
// motivo em português, para ser mostrado ao usuário sem que ele precise
// entender a classificação página a página.
type FileEligibility struct {
	Path     string
	Pages    []PageInfo
	Eligible bool
	// Reason é vazio quando Eligible; caso contrário, explica em
	// português por que o arquivo foi recusado.
	Reason string
}

// DecideEligibility decide se um arquivo pode ser processado por ocr-pdf a
// partir das páginas já classificadas (ver ClassifyPages). Função pura,
// sem I/O — testável sem tocar em PDF nenhum.
//
// A regra é deliberadamente conservadora (ver AGENTS.md): ocr-pdf
// reconstrói o PDF de saída a partir das imagens extraídas das páginas, o
// que é fiel quando toda página é puro scan e destrutivo quando não é.
// Destruir conteúdo em silêncio seria muito pior que recusar o arquivo, e
// por isso qualquer combinação diferente de "toda página é PagePureScan" é
// recusada, com um motivo específico:
//
//   - zero páginas: nada a fazer.
//   - alguma página PageMixed: reconstruir perderia o conteúdo que não é a
//     imagem única daquela página (texto, ou imagens extras).
//   - TODAS as páginas já têm texto (PageHasText): o arquivo já é
//     pesquisável — não é erro, é economia (não há o que reconhecer).
//   - mistura de PagePureScan e PageHasText: só parte do arquivo é
//     digitalizada; reconstruir perderia o texto das páginas que já têm.
//   - alguma PageNoImage junto de páginas de scan: mesmo raciocínio — não
//     há como reconstruir aquela página a partir de imagem nenhuma.
func DecideEligibility(path string, pages []PageInfo) FileEligibility {
	fe := FileEligibility{Path: path, Pages: pages}

	if len(pages) == 0 {
		fe.Reason = "o arquivo não tem nenhuma página"
		return fe
	}

	var pureScan, hasText, mixed, noImage int
	for _, p := range pages {
		switch p.Kind {
		case PagePureScan:
			pureScan++
		case PageHasText:
			hasText++
		case PageMixed:
			mixed++
		case PageNoImage:
			noImage++
		}
	}

	if mixed > 0 {
		fe.Reason = fmt.Sprintf(
			"pelo menos uma de %d página(s) tem conteúdo além de uma única imagem (texto embutido junto de imagem, ou mais de uma imagem na mesma página); reconstruir o PDF a partir das imagens perderia esse conteúdo",
			len(pages),
		)
		return fe
	}

	if hasText == len(pages) {
		fe.Reason = "o arquivo já tem texto embutido em todas as páginas; não é um PDF digitalizado, não há o que reconhecer"
		return fe
	}

	if hasText > 0 {
		fe.Reason = fmt.Sprintf(
			"só parte do arquivo é digitalizada (%d de %d páginas já têm texto embutido); reconstruir o PDF a partir das imagens perderia o texto das páginas restantes",
			hasText, len(pages),
		)
		return fe
	}

	if noImage > 0 {
		fe.Reason = fmt.Sprintf(
			"%d de %d página(s) não têm imagem nem texto; não é seguro reconstruir o arquivo a partir das imagens",
			noImage, len(pages),
		)
		return fe
	}

	// Sobrou só a combinação segura: toda página é PagePureScan.
	fe.Eligible = true
	return fe
}

// OCRizeOptions descreve os parâmetros de uma execução de OCRize.
type OCRizeOptions struct {
	// Inputs são os arquivos PDF e/ou pastas a considerar. Resolvido com
	// ResolveInputs, igual a merge-pdf/organize-pdf.
	Inputs   []string
	MaxDepth int
	// OutputDir é a pasta de saída; vazio grava ao lado de cada original.
	OutputDir string
	// Suffix é o sufixo do arquivo gerado; vazio usa "-ocr".
	Suffix string
	// Lang é o idioma do OCR; vazio usa "por".
	Lang string
	// Overwrite permite sobrescrever um arquivo de saída já existente.
	Overwrite bool
	// SkipExisting pula (sem erro) um arquivo cuja saída já existe —
	// pensado para retomar um lote grande interrompido no meio.
	SkipExisting bool
	// DryRun faz toda a classificação e decisão de elegibilidade, sem
	// gerar nenhum arquivo.
	DryRun bool
	// Engine é o motor de OCR→PDF pesquisável. Obrigatório e precisa
	// estar disponível: ao contrário do OCR usado como fallback de
	// leitura em outras ferramentas, ocr-pdf EXIGE o motor (é o próprio
	// propósito da ferramenta).
	Engine SearchablePDFEngine
	// Progress, quando não-nil, é chamado uma vez por arquivo processado
	// (elegível ou não), depois de decidido o resultado dele.
	Progress func(done, total int, path string)
}

// OCRizeEntry descreve o resultado (ou a tentativa) de processar um único
// arquivo.
type OCRizeEntry struct {
	Source string
	// Dest é o caminho calculado de saída — preenchido mesmo quando o
	// arquivo não foi processado, para fins de relatório.
	Dest string
	// Pages é a quantidade de páginas do arquivo de origem.
	Pages int
	// Done indica se o arquivo foi (ou, em DryRun, seria) processado.
	Done bool
	// Reason é vazio quando Done; caso contrário, explica por que o
	// arquivo foi pulado.
	Reason string
}

// OCRizeResult descreve o resultado de uma execução de OCRize.
type OCRizeResult struct {
	Processed []OCRizeEntry
	Skipped   []OCRizeEntry
	DryRun    bool
	Total     int
	// Warnings acumula avisos não-fatais (ex: imagem extraída cujo nome
	// não pôde ser associada a uma página — ver ParseExtractedImageName).
	Warnings []string
}

// Summary devolve um resumo textual curto do resultado.
func (r OCRizeResult) Summary() string {
	prefix := ""
	if r.DryRun {
		prefix = "[simulação] "
	}
	return fmt.Sprintf("%s%d de %d arquivos processados, %d pulados", prefix, len(r.Processed), r.Total, len(r.Skipped))
}

// OCRizeReportRow é uma linha do relatório de uma execução de OCRize —
// mesmo espírito de ReportRow (report.go), adaptado às colunas próprias de
// ocr-pdf.
type OCRizeReportRow struct {
	Arquivo    string `json:"arquivo"`
	Origem     string `json:"origem"`
	Destino    string `json:"destino"`
	Processado bool   `json:"processado"`
	Paginas    int    `json:"paginas"`
	Motivo     string `json:"motivo"`
}

// BuildOCRizeReport monta as linhas do relatório de uma execução a partir
// do seu OCRizeResult, incluindo tanto os arquivos processados quanto os
// pulados. Função pura, sem I/O — mesmo padrão de BuildReport (report.go).
// As linhas saem ordenadas por Arquivo, para que duas execuções do mesmo
// lote sejam comparáveis lado a lado.
func BuildOCRizeReport(r OCRizeResult) []OCRizeReportRow {
	rows := make([]OCRizeReportRow, 0, len(r.Processed)+len(r.Skipped))

	for _, e := range r.Processed {
		rows = append(rows, OCRizeReportRow{
			Arquivo:    filepath.Base(e.Source),
			Origem:     e.Source,
			Destino:    e.Dest,
			Processado: true,
			Paginas:    e.Pages,
		})
	}
	for _, e := range r.Skipped {
		rows = append(rows, OCRizeReportRow{
			Arquivo:    filepath.Base(e.Source),
			Origem:     e.Source,
			Destino:    e.Dest,
			Processado: false,
			Paginas:    e.Pages,
			Motivo:     e.Reason,
		})
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Arquivo < rows[j].Arquivo })

	return rows
}

// PageCount devolve a quantidade de páginas de um PDF. Usado por
// internal/tools/ocrpdf para compor a linha de progresso ("[3/120]
// nota-003.pdf — 2 página(s)...") sem que a ferramenta precise importar
// pdfcpu diretamente — o mesmo raciocínio já aplicado ao resto do pacote:
// pdfutil é o único ponto do CLI que conhece a biblioteca de manipulação de
// PDF.
func PageCount(path string) (int, error) {
	return api.PageCountFile(path)
}

// OCRize processa os arquivos resolvidos a partir de opts.Inputs: cada
// arquivo cujas páginas sejam TODAS puro scan (ver DecideEligibility) tem
// um PDF pesquisável gerado ao lado (ou em opts.OutputDir), nunca
// sobrescrevendo o original; os demais são pulados com um motivo claro.
func OCRize(ctx context.Context, opts OCRizeOptions) (OCRizeResult, error) {
	if err := ctx.Err(); err != nil {
		return OCRizeResult{}, err
	}

	if opts.Engine == nil || !opts.Engine.Available() {
		return OCRizeResult{}, fmt.Errorf("ocrize: motor de OCR indisponível")
	}

	suffix := opts.Suffix
	if suffix == "" {
		suffix = "-ocr"
	}
	lang := opts.Lang
	if lang == "" {
		lang = "por"
	}

	files, err := ResolveInputs(opts.Inputs, opts.MaxDepth, "name")
	if err != nil {
		return OCRizeResult{}, err
	}

	result := OCRizeResult{DryRun: opts.DryRun, Total: len(files)}

	for i, file := range files {
		if err := ctx.Err(); err != nil {
			return OCRizeResult{}, err
		}

		entry, warnings, err := ocrizeOneFile(ctx, file, suffix, lang, opts)
		if err != nil {
			// Só chega aqui quando o contexto foi cancelado no meio do
			// processamento (entre páginas) — qualquer outra falha é
			// tratada dentro de ocrizeOneFile e vira um Skipped com
			// motivo, nunca interrompe o lote inteiro.
			return OCRizeResult{}, err
		}
		result.Warnings = append(result.Warnings, warnings...)

		if entry.Done {
			result.Processed = append(result.Processed, entry)
		} else {
			result.Skipped = append(result.Skipped, entry)
		}

		if opts.Progress != nil {
			opts.Progress(i+1, len(files), file)
		}
	}

	return result, nil
}

// ocrizeOneFile processa (ou classifica, em DryRun) um único arquivo. O
// erro devolvido só é não-nil quando o contexto foi cancelado durante o
// processamento — qualquer outra falha (extração, OCR, geração de PDF)
// vira um OCRizeEntry com Done=false e Reason preenchido, sem propagar
// erro: falha num arquivo do lote nunca derruba os demais.
func ocrizeOneFile(ctx context.Context, srcPath, suffix, lang string, opts OCRizeOptions) (OCRizeEntry, []string, error) {
	entry := OCRizeEntry{Source: srcPath}

	outDir := opts.OutputDir
	if outDir == "" {
		outDir = filepath.Dir(srcPath)
	}
	base := strings.TrimSuffix(filepath.Base(srcPath), filepath.Ext(srcPath))
	dest := filepath.Join(outDir, base+suffix+".pdf")
	entry.Dest = dest

	// Nunca sobrescreve o original: se o sufixo/pasta de saída resolverem
	// para o mesmo caminho do arquivo de origem (ex: --suffix "" sem
	// --output-dir), recusa explicitamente em vez de gravar por cima.
	srcAbs, srcAbsErr := filepath.Abs(srcPath)
	destAbs, destAbsErr := filepath.Abs(dest)
	if srcAbsErr == nil && destAbsErr == nil && filepath.Clean(srcAbs) == filepath.Clean(destAbs) {
		entry.Reason = "o destino calculado é igual ao arquivo original; ajuste --suffix ou --output-dir"
		return entry, nil, nil
	}

	tmpDir, err := os.MkdirTemp("", "ocrize-*")
	if err != nil {
		entry.Reason = fmt.Sprintf("não foi possível criar diretório temporário: %v", err)
		return entry, nil, nil
	}
	defer os.RemoveAll(tmpDir)

	if err := api.ExtractImagesFile(srcPath, tmpDir, nil, nil); err != nil {
		entry.Reason = fmt.Sprintf("não foi possível extrair imagens do arquivo: %v", err)
		return entry, nil, nil
	}

	dirEntries, err := os.ReadDir(tmpDir)
	if err != nil {
		entry.Reason = fmt.Sprintf("não foi possível ler imagens extraídas: %v", err)
		return entry, nil, nil
	}

	imagesByPage := make(map[int][]extractedImage)
	imageCountByPage := make(map[int]int)
	var warnings []string
	for _, de := range dirEntries {
		if de.IsDir() {
			continue
		}
		name := de.Name()
		page, index, ok := ParseExtractedImageName(name)
		if !ok {
			warnings = append(warnings, fmt.Sprintf(
				"%s: imagem extraída %q não pôde ser associada a nenhuma página e foi ignorada",
				filepath.Base(srcPath), name,
			))
			continue
		}
		imageCountByPage[page]++
		imagesByPage[page] = append(imagesByPage[page], extractedImage{index: index, path: filepath.Join(tmpDir, name)})
	}

	pageTexts, err := ExtractPageTexts(srcPath)
	if err != nil {
		entry.Reason = fmt.Sprintf("não foi possível extrair o texto embutido do arquivo: %v", err)
		return entry, warnings, nil
	}
	entry.Pages = len(pageTexts)

	pages := ClassifyPages(pageTexts, imageCountByPage)
	elig := DecideEligibility(srcPath, pages)
	if !elig.Eligible {
		entry.Reason = elig.Reason
		return entry, warnings, nil
	}

	if _, statErr := os.Stat(dest); statErr == nil {
		if opts.SkipExisting {
			entry.Reason = "arquivo de saída já existe (pulado por --skip-existing)"
			return entry, warnings, nil
		}
		if !opts.Overwrite {
			entry.Reason = "arquivo de saída já existe; use --overwrite para sobrescrever ou --skip-existing para pular sem erro"
			return entry, warnings, nil
		}
	}

	if opts.DryRun {
		entry.Done = true
		return entry, warnings, nil
	}

	// Gera o PDF pesquisável de cada página, na ORDEM DO NÚMERO DA
	// PÁGINA — nunca na ordem alfabética do nome do arquivo de imagem
	// extraída, que não segue necessariamente a ordem numérica (ex:
	// "..._1_..." antes de "..._10_..." alfabeticamente, mas depois
	// numericamente). Os nomes dos PDFs por página são gerados aqui, com
	// zero-padding, então mesmo a ordenação alfabética de Merge (abaixo)
	// preserva a ordem correta.
	pageNumbers := make([]int, 0, len(pages))
	for _, p := range pages {
		pageNumbers = append(pageNumbers, p.Number)
	}
	sort.Ints(pageNumbers)

	pagePDFs := make([]string, 0, len(pageNumbers))
	for _, pn := range pageNumbers {
		if err := ctx.Err(); err != nil {
			return entry, warnings, err
		}

		imgs := imagesByPage[pn]
		if len(imgs) != 1 {
			entry.Reason = fmt.Sprintf("página %d: esperava exatamente 1 imagem, encontrou %d", pn, len(imgs))
			return entry, warnings, nil
		}

		outBase := filepath.Join(tmpDir, fmt.Sprintf("pagina-%05d", pn))
		if err := opts.Engine.ImageToSearchablePDF(ctx, imgs[0].path, outBase, lang); err != nil {
			entry.Reason = fmt.Sprintf("falha no OCR da página %d: %v", pn, err)
			return entry, warnings, nil
		}
		pagePDFs = append(pagePDFs, outBase+".pdf")
	}

	if dir := filepath.Dir(dest); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			entry.Reason = fmt.Sprintf("não foi possível criar a pasta de destino: %v", err)
			return entry, warnings, nil
		}
	}

	if _, err := Merge(ctx, MergeOptions{
		Inputs:    pagePDFs,
		Output:    dest,
		Sort:      "name",
		Overwrite: true, // colisão já verificada acima
	}); err != nil {
		entry.Reason = fmt.Sprintf("não foi possível unir as páginas geradas: %v", err)
		return entry, warnings, nil
	}

	entry.Done = true
	return entry, warnings, nil
}
