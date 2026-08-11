// Package pdfutil implementa o núcleo de manipulação de PDF do CLI:
// união, separação, organização e extração de texto.
package pdfutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/ledongthuc/pdf"
	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// OCREngine é o motor de OCR. A interface é declarada aqui (e não importada
// do pacote de OCR) para manter pdfutil testável com um motor falso e sem
// dependência de processo externo nos testes.
type OCREngine interface {
	Available() bool
	ImageToText(ctx context.Context, imagePath, lang string) (string, error)
}

// OCRMode controla quando o OCR é usado como fallback da extração de texto.
type OCRMode string

const (
	OCRAuto   OCRMode = "auto"   // usa OCR só nas páginas sem texto embutido
	OCRAlways OCRMode = "always" // usa OCR em todas as páginas, ignorando o texto embutido
	OCRNever  OCRMode = "never"  // nunca usa OCR (comportamento anterior)
)

// ParseOCRMode valida a string vinda da flag --ocr.
// Vazio devolve OCRAuto. Valor inválido devolve erro em português listando os
// válidos. Aceita também os sinônimos em português "sempre" (-> always) e
// "nunca" (-> never), por conveniência de quem escreve a flag.
func ParseOCRMode(s string) (OCRMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return OCRAuto, nil
	case "auto":
		return OCRAuto, nil
	case "always", "sempre":
		return OCRAlways, nil
	case "never", "nunca":
		return OCRNever, nil
	default:
		return "", fmt.Errorf("modo de OCR inválido %q: valores aceitos são \"auto\", \"always\" (ou \"sempre\"), \"never\" (ou \"nunca\")", s)
	}
}

// TextOptions controla o comportamento de ExtractPageTextsOpts/ExtractTextOpts.
type TextOptions struct {
	Mode   OCRMode
	Lang   string    // idioma do OCR; vazio = "por"
	Engine OCREngine // nil = OCR indisponível
}

// --- cache de textos extraídos ---------------------------------------------

// textCacheKey identifica de forma estável o resultado de uma extração para
// um arquivo específico, nas opções usadas. ModTime + tamanho detectam
// alteração do arquivo entre chamadas; Mode/Lang distinguem resultados que
// dependem de OCR ou de idioma diferentes.
type textCacheKey struct {
	path    string
	modTime int64
	size    int64
	mode    OCRMode
	lang    string
}

// textCacheEntry guarda, junto do texto extraído, os avisos gerados durante
// a extração (ex: imagem cujo nome não pôde ser associada a uma página) —
// para que uma segunda chamada servida pelo cache continue reportando os
// mesmos avisos da primeira, em vez de silenciá-los.
type textCacheEntry struct {
	texts    []string
	warnings []string
}

var (
	textCacheMu sync.RWMutex
	textCache   = map[textCacheKey]textCacheEntry{}
)

// ClearTextCache limpa o cache de textos extraídos (usado em teste).
func ClearTextCache() {
	textCacheMu.Lock()
	defer textCacheMu.Unlock()
	textCache = map[textCacheKey]textCacheEntry{}
}

func cacheGet(key textCacheKey) (textCacheEntry, bool) {
	textCacheMu.RLock()
	defer textCacheMu.RUnlock()
	v, ok := textCache[key]
	return v, ok
}

func cacheSet(key textCacheKey, entry textCacheEntry) {
	textCacheMu.Lock()
	defer textCacheMu.Unlock()
	textCache[key] = entry
}

// --- extração de texto embutido ---------------------------------------------

// extractEmbeddedPageTexts devolve o texto embutido de cada página do PDF em
// path, usando github.com/ledongthuc/pdf. É a lógica que já existia antes do
// fallback de OCR.
func extractEmbeddedPageTexts(path string) ([]string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("abrir PDF %q: %w", path, err)
	}
	defer f.Close()

	numPages := r.NumPage()
	texts := make([]string, 0, numPages)

	for i := 1; i <= numPages; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			texts = append(texts, "")
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			// Página com falha de extração não deve abortar o documento
			// inteiro: registramos texto vazio e seguimos.
			texts = append(texts, "")
			continue
		}
		texts = append(texts, text)
	}

	return texts, nil
}

// ExtractPageTexts devolve o texto de cada página do PDF em path.
// O índice 0 do slice retornado corresponde à página 1 do documento.
//
// Mantida por compatibilidade: delega para ExtractPageTextsOpts com
// Mode: OCRNever, ou seja, comportamento idêntico ao anterior (sem OCR).
func ExtractPageTexts(path string) ([]string, error) {
	texts, _, err := ExtractPageTextsOpts(context.Background(), path, TextOptions{Mode: OCRNever})
	return texts, err
}

// ExtractText devolve o texto do documento inteiro em path, com o texto de
// cada página concatenado por "\n".
//
// Mantida por compatibilidade: delega para ExtractTextOpts com
// Mode: OCRNever, ou seja, comportamento idêntico ao anterior (sem OCR).
func ExtractText(path string) (string, error) {
	text, _, err := ExtractTextOpts(context.Background(), path, TextOptions{Mode: OCRNever})
	return text, err
}

// extractedImageNamePattern casa nomes gerados pelo pdfcpu no formato
// "<baseDoPDF>_<pagina>_<prefixo><indice>.<ext>". O nome base do PDF pode
// ele próprio conter "_" e dígitos (ex.: "nota_2024_01.pdf"), então o ".*"
// guloso no início garante que os dois últimos grupos numéricos — os que
// realmente importam — sejam capturados, não os primeiros que aparecerem.
//
// O prefixo do último segmento NÃO é sempre "Im": o pdfcpu deriva esse nome
// do nome do recurso XObject da página (ver WriteImageToDisk em
// pdfcpu/pkg/api/extract.go, campo img.Name), então varia conforme como o
// PDF nomeia a imagem — "Im0", "X0", "Fm2", etc. Travar em "Im" fazia
// páginas inteiras serem descartadas do OCR em silêncio sempre que o pdfcpu
// usasse outro prefixo (ver AGENTS.md). Por isso o padrão aceita qualquer
// sequência de letras no lugar de "Im".
var extractedImageNamePattern = regexp.MustCompile(`^.*_(\d+)_[A-Za-z]+(\d+)\.[A-Za-z0-9]+$`)

// ParseExtractedImageName extrai o número da página e o índice da imagem de
// um nome gerado pelo pdfcpu (api.ExtractImagesFile). ok=false se o nome não
// casar com o padrão "<base>_<pagina>_<prefixo><indice>.<ext>".
func ParseExtractedImageName(filename string) (page, index int, ok bool) {
	m := extractedImageNamePattern.FindStringSubmatch(filename)
	if m == nil {
		return 0, 0, false
	}
	p, errP := strconv.Atoi(m[1])
	i, errI := strconv.Atoi(m[2])
	if errP != nil || errI != nil {
		return 0, 0, false
	}
	return p, i, true
}

// extractedImage associa um caminho de arquivo de imagem extraída ao seu
// índice dentro da página (para ordenação estável).
type extractedImage struct {
	index int
	path  string
}

// ExtractPageTextsOpts extrai o texto de cada página do PDF em path,
// aplicando OCR conforme opts. O índice 0 do slice retornado corresponde à
// página 1 do documento.
//
// O segundo valor devolvido lista avisos não-fatais ocorridos durante a
// extração — hoje, só imagens extraídas para OCR cujo nome não pôde ser
// associado a uma página (ver ParseExtractedImageName). A extração em si não
// falha por causa deles: o texto das demais páginas continua sendo
// devolvido normalmente, mas quem chama precisa repassar os avisos adiante
// (ver OrganizeResult.Warnings / SplitResult.Warnings) para que o usuário
// saiba que alguma página pode ter ficado sem OCR.
func ExtractPageTextsOpts(ctx context.Context, path string, opts TextOptions) ([]string, []string, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	info, statErr := os.Stat(path)
	var cacheKey textCacheKey
	cacheable := statErr == nil
	if cacheable {
		cacheKey = textCacheKey{
			path:    path,
			modTime: info.ModTime().UnixNano(),
			size:    info.Size(),
			mode:    opts.Mode,
			lang:    opts.Lang,
		}
		if cached, ok := cacheGet(cacheKey); ok {
			return cached.texts, cached.warnings, nil
		}
	}

	texts, err := extractEmbeddedPageTexts(path)
	if err != nil {
		return nil, nil, err
	}

	result, warnings, err := applyOCRFallback(ctx, path, texts, opts)
	if err != nil {
		return nil, nil, err
	}

	if cacheable {
		cacheSet(cacheKey, textCacheEntry{texts: result, warnings: warnings})
	}

	return result, warnings, nil
}

// applyOCRFallback decide, a partir de opts e do texto embutido já extraído,
// se e quais páginas precisam de OCR, roda o motor sobre as imagens
// extraídas dessas páginas, e devolve o texto final por página, junto dos
// avisos não-fatais ocorridos durante a extração de imagens (ver
// ExtractPageTextsOpts).
func applyOCRFallback(ctx context.Context, path string, embedded []string, opts TextOptions) ([]string, []string, error) {
	if opts.Mode == OCRNever || opts.Engine == nil || !opts.Engine.Available() {
		return embedded, nil, nil
	}

	pagesNeedingOCR := make([]int, 0) // 1-based
	switch opts.Mode {
	case OCRAlways:
		for i := range embedded {
			pagesNeedingOCR = append(pagesNeedingOCR, i+1)
		}
	default: // OCRAuto
		for i, t := range embedded {
			if strings.TrimSpace(t) == "" {
				pagesNeedingOCR = append(pagesNeedingOCR, i+1)
			}
		}
	}

	if len(pagesNeedingOCR) == 0 {
		return embedded, nil, nil
	}

	needsOCR := make(map[int]bool, len(pagesNeedingOCR))
	for _, p := range pagesNeedingOCR {
		needsOCR[p] = true
	}

	outDir, err := os.MkdirTemp("", "pdfutil-ocr-*")
	if err != nil {
		return nil, nil, fmt.Errorf("criar diretório temporário para OCR: %w", err)
	}
	defer os.RemoveAll(outDir)

	if err := api.ExtractImagesFile(path, outDir, nil, nil); err != nil {
		return nil, nil, fmt.Errorf("extrair imagens de %q para OCR: %w", path, err)
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		return nil, nil, fmt.Errorf("ler diretório de imagens extraídas %q: %w", outDir, err)
	}

	// warnings acumula avisos não-fatais desta extração. O caso central: o
	// pdfcpu deriva o nome de cada imagem extraída do nome do recurso
	// XObject da página, e esse nome nem sempre segue o prefixo "Im" (ver
	// extractedImageNamePattern). Antes desta checagem, um arquivo cujo nome
	// não casasse com o padrão era simplesmente pulado — a página
	// correspondente ficava sem OCR e ninguém era avisado. É esse silêncio,
	// não a expressão regular em si, que permitiu o defeito atravessar seis
	// versões sem ser notado (ver AGENTS.md).
	var warnings []string
	imageFileCount := 0
	matchedFileCount := 0

	imagesByPage := make(map[int][]extractedImage)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		imageFileCount++
		name := entry.Name()
		page, index, ok := ParseExtractedImageName(name)
		if !ok {
			warnings = append(warnings, fmt.Sprintf(
				"imagem extraída %q não pôde ser associada a nenhuma página (nome fora do padrão esperado) e foi ignorada no OCR",
				name,
			))
			continue
		}
		matchedFileCount++
		if !needsOCR[page] {
			continue
		}
		imagesByPage[page] = append(imagesByPage[page], extractedImage{
			index: index,
			path:  filepath.Join(outDir, name),
		})
	}

	// Sinal forte de que o pdfcpu mudou de novo a convenção de nomes: imagens
	// foram extraídas, mas NENHUMA pôde ser associada a uma página. Além dos
	// avisos individuais acima (um por arquivo), isto merece um aviso à
	// parte, explícito sobre a causa provável.
	if imageFileCount > 0 && matchedFileCount == 0 {
		warnings = append(warnings, fmt.Sprintf(
			"nenhuma das %d imagem(ns) extraída(s) de %q pôde ser associada a uma página; o pdfcpu pode ter mudado a convenção de nomes dos arquivos extraídos e nenhuma página foi enviada ao OCR",
			imageFileCount, path,
		))
	}

	lang := opts.Lang
	if lang == "" {
		lang = "por"
	}

	result := make([]string, len(embedded))
	copy(result, embedded)

	for _, page := range pagesNeedingOCR {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}

		images := imagesByPage[page]
		if len(images) == 0 {
			continue
		}
		sort.Slice(images, func(i, j int) bool { return images[i].index < images[j].index })

		var parts []string
		for _, img := range images {
			text, err := opts.Engine.ImageToText(ctx, img.path, lang)
			if err != nil {
				// Erro pontual de OCR numa imagem não pode abortar o
				// documento inteiro: registramos e seguimos para a próxima.
				continue
			}
			parts = append(parts, text)
		}
		if len(parts) > 0 {
			result[page-1] = strings.Join(parts, "\n")
		}
	}

	return result, warnings, nil
}

// ExtractTextOpts devolve o texto do documento inteiro em path, com o texto
// de cada página (após aplicado o fallback de OCR conforme opts)
// concatenado por "\n", e os avisos não-fatais ocorridos durante a extração
// (ver ExtractPageTextsOpts).
func ExtractTextOpts(ctx context.Context, path string, opts TextOptions) (string, []string, error) {
	pages, warnings, err := ExtractPageTextsOpts(ctx, path, opts)
	if err != nil {
		return "", nil, err
	}
	return strings.Join(pages, "\n"), warnings, nil
}
