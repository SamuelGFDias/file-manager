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

var (
	textCacheMu sync.RWMutex
	textCache   = map[textCacheKey][]string{}
)

// ClearTextCache limpa o cache de textos extraídos (usado em teste).
func ClearTextCache() {
	textCacheMu.Lock()
	defer textCacheMu.Unlock()
	textCache = map[textCacheKey][]string{}
}

func cacheGet(key textCacheKey) ([]string, bool) {
	textCacheMu.RLock()
	defer textCacheMu.RUnlock()
	v, ok := textCache[key]
	return v, ok
}

func cacheSet(key textCacheKey, texts []string) {
	textCacheMu.Lock()
	defer textCacheMu.Unlock()
	textCache[key] = texts
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
	return ExtractPageTextsOpts(context.Background(), path, TextOptions{Mode: OCRNever})
}

// ExtractText devolve o texto do documento inteiro em path, com o texto de
// cada página concatenado por "\n".
//
// Mantida por compatibilidade: delega para ExtractTextOpts com
// Mode: OCRNever, ou seja, comportamento idêntico ao anterior (sem OCR).
func ExtractText(path string) (string, error) {
	return ExtractTextOpts(context.Background(), path, TextOptions{Mode: OCRNever})
}

// extractedImageNamePattern casa nomes gerados pelo pdfcpu no formato
// "<baseDoPDF>_<pagina>_Im<indice>.<ext>". O nome base do PDF pode ele
// próprio conter "_" e dígitos (ex.: "nota_2024_01.pdf"), então o ".*"
// guloso no início garante que os dois últimos grupos numéricos — os que
// realmente importam — sejam capturados, não os primeiros que aparecerem.
var extractedImageNamePattern = regexp.MustCompile(`^.*_(\d+)_Im(\d+)\.[A-Za-z0-9]+$`)

// ParseExtractedImageName extrai o número da página e o índice da imagem de
// um nome gerado pelo pdfcpu (api.ExtractImagesFile). ok=false se o nome não
// casar com o padrão "<base>_<pagina>_Im<indice>.<ext>".
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
func ExtractPageTextsOpts(ctx context.Context, path string, opts TextOptions) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
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
			return cached, nil
		}
	}

	texts, err := extractEmbeddedPageTexts(path)
	if err != nil {
		return nil, err
	}

	result, err := applyOCRFallback(ctx, path, texts, opts)
	if err != nil {
		return nil, err
	}

	if cacheable {
		cacheSet(cacheKey, result)
	}

	return result, nil
}

// applyOCRFallback decide, a partir de opts e do texto embutido já extraído,
// se e quais páginas precisam de OCR, roda o motor sobre as imagens
// extraídas dessas páginas, e devolve o texto final por página.
func applyOCRFallback(ctx context.Context, path string, embedded []string, opts TextOptions) ([]string, error) {
	if opts.Mode == OCRNever || opts.Engine == nil || !opts.Engine.Available() {
		return embedded, nil
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
		return embedded, nil
	}

	needsOCR := make(map[int]bool, len(pagesNeedingOCR))
	for _, p := range pagesNeedingOCR {
		needsOCR[p] = true
	}

	outDir, err := os.MkdirTemp("", "pdfutil-ocr-*")
	if err != nil {
		return nil, fmt.Errorf("criar diretório temporário para OCR: %w", err)
	}
	defer os.RemoveAll(outDir)

	if err := api.ExtractImagesFile(path, outDir, nil, nil); err != nil {
		return nil, fmt.Errorf("extrair imagens de %q para OCR: %w", path, err)
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		return nil, fmt.Errorf("ler diretório de imagens extraídas %q: %w", outDir, err)
	}

	imagesByPage := make(map[int][]extractedImage)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		page, index, ok := ParseExtractedImageName(name)
		if !ok || !needsOCR[page] {
			continue
		}
		imagesByPage[page] = append(imagesByPage[page], extractedImage{
			index: index,
			path:  filepath.Join(outDir, name),
		})
	}

	lang := opts.Lang
	if lang == "" {
		lang = "por"
	}

	result := make([]string, len(embedded))
	copy(result, embedded)

	for _, page := range pagesNeedingOCR {
		if err := ctx.Err(); err != nil {
			return nil, err
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

	return result, nil
}

// ExtractTextOpts devolve o texto do documento inteiro em path, com o texto
// de cada página (após aplicado o fallback de OCR conforme opts)
// concatenado por "\n".
func ExtractTextOpts(ctx context.Context, path string, opts TextOptions) (string, error) {
	pages, err := ExtractPageTextsOpts(ctx, path, opts)
	if err != nil {
		return "", err
	}
	return strings.Join(pages, "\n"), nil
}
