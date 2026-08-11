package pdfutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// SplitMode define a estratégia usada para separar um PDF em vários arquivos.
type SplitMode string

const (
	SplitByPage  SplitMode = "page"
	SplitByRange SplitMode = "range"
	SplitByRegex SplitMode = "regex"
)

// SplitOptions descreve os parâmetros de uma operação de separação de PDF.
type SplitOptions struct {
	Input        string
	OutputDir    string
	Mode         SplitMode
	Ranges       []string       // modo range, ex ["1-5","6-10","11-"]
	Regex        *regexp.Regexp // modo regex
	NameTemplate string         // ex "pagina-%03d.pdf"; usado quando não há captura
	Overwrite    bool
}

// SplitResult descreve o resultado de uma operação de separação de PDF.
type SplitResult struct {
	Outputs  []string
	Warnings []string
}

// PageGroup representa um intervalo contíguo de páginas que formará um
// arquivo de saída no modo regex.
type PageGroup struct {
	Name  string // nome do arquivo SEM extensão
	Start int    // 1-indexed, inclusivo
	End   int    // 1-indexed, inclusivo
}

var invalidFilenameChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1F\x7F]`)

// SanitizeFilename remove de s qualquer separador de caminho, sequências
// ".." (path traversal), caracteres inválidos em nomes de arquivo no Windows
// e caracteres de controle, substituindo-os por "_". Espaços e pontos nas
// pontas são removidos. O resultado pode ser uma string vazia, caso em que o
// chamador deve usar um nome alternativo.
func SanitizeFilename(s string) string {
	for strings.Contains(s, "..") {
		s = strings.ReplaceAll(s, "..", "_")
	}
	s = invalidFilenameChars.ReplaceAllString(s, "_")
	s = strings.Trim(s, " .")
	return s
}

var rangeItemRegex = regexp.MustCompile(`^\d+$|^\d+-$|^\d+-\d+$|^-\d+$`)

// ParseRanges converte uma especificação como "1-5,6-10,11-" em uma lista de
// itens no formato aceito por selectedPages do pdfcpu.
func ParseRanges(spec string) ([]string, error) {
	parts := strings.Split(spec, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		if !rangeItemRegex.MatchString(item) {
			return nil, fmt.Errorf("intervalo de páginas inválido: %q", item)
		}
		result = append(result, item)
	}
	return result, nil
}

// GroupPagesByRegex agrupa as páginas (pageTexts, índice 0 = página 1) em
// grupos contíguos, onde cada página cujo texto casa com re inicia um novo
// grupo. Páginas que não casam pertencem ao grupo atualmente aberto.
func GroupPagesByRegex(pageTexts []string, re *regexp.Regexp, nameTemplate string) []PageGroup {
	if len(pageTexts) == 0 {
		return nil
	}
	if nameTemplate == "" {
		nameTemplate = "documento-%03d"
	}

	type segment struct {
		start   int
		end     int
		capture string
		hasCap  bool
	}

	var segments []segment
	currentStart := 1
	var currentCapture string
	var currentHasCap bool

	for i, text := range pageTexts {
		pageNum := i + 1
		if !re.MatchString(text) {
			continue
		}
		if pageNum > currentStart {
			segments = append(segments, segment{
				start:   currentStart,
				end:     pageNum - 1,
				capture: currentCapture,
				hasCap:  currentHasCap,
			})
		}
		currentStart = pageNum
		m := re.FindStringSubmatch(text)
		if len(m) > 1 && m[1] != "" {
			currentCapture = m[1]
			currentHasCap = true
		} else {
			currentCapture = ""
			currentHasCap = false
		}
	}
	segments = append(segments, segment{
		start:   currentStart,
		end:     len(pageTexts),
		capture: currentCapture,
		hasCap:  currentHasCap,
	})

	groups := make([]PageGroup, 0, len(segments))
	usedNames := make(map[string]int)

	for idx, seg := range segments {
		n := idx + 1
		name := ""
		if seg.hasCap {
			name = SanitizeFilename(seg.capture)
		}
		if name == "" {
			name = fmt.Sprintf(nameTemplate, n)
		}

		count := usedNames[name]
		finalName := name
		if count > 0 {
			finalName = fmt.Sprintf("%s-%d", name, count+1)
		}
		usedNames[name] = count + 1

		groups = append(groups, PageGroup{
			Name:  finalName,
			Start: seg.start,
			End:   seg.end,
		})
	}

	return groups
}

// listDirNames devolve o conjunto de nomes de arquivos (não diretórios)
// presentes em dir.
func listDirNames(dir string) (map[string]struct{}, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("listar diretório %q: %w", dir, err)
	}
	names := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		names[e.Name()] = struct{}{}
	}
	return names, nil
}

// Split separa opts.Input em vários arquivos de acordo com opts.Mode.
func Split(ctx context.Context, opts SplitOptions) (SplitResult, error) {
	if err := ctx.Err(); err != nil {
		return SplitResult{}, err
	}

	if opts.Input == "" {
		return SplitResult{}, fmt.Errorf("arquivo de entrada não informado")
	}

	info, err := os.Stat(opts.Input)
	if err != nil {
		return SplitResult{}, fmt.Errorf("arquivo de entrada %q não existe: %w", opts.Input, err)
	}
	if info.IsDir() {
		return SplitResult{}, fmt.Errorf("entrada %q é um diretório; informe um arquivo PDF", opts.Input)
	}
	if !strings.EqualFold(filepath.Ext(opts.Input), ".pdf") {
		return SplitResult{}, fmt.Errorf("arquivo de entrada %q não é um PDF", opts.Input)
	}

	outDir := opts.OutputDir
	if outDir == "" {
		outDir = filepath.Dir(opts.Input)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return SplitResult{}, fmt.Errorf("criar diretório de saída %q: %w", outDir, err)
	}

	result := SplitResult{}

	switch opts.Mode {
	case SplitByPage:
		before, err := listDirNames(outDir)
		if err != nil {
			return SplitResult{}, err
		}
		if err := api.SplitFile(opts.Input, outDir, 1, nil); err != nil {
			return SplitResult{}, fmt.Errorf("separar %q por página: %w", opts.Input, err)
		}
		after, err := listDirNames(outDir)
		if err != nil {
			return SplitResult{}, err
		}
		for name := range after {
			if _, existed := before[name]; !existed {
				result.Outputs = append(result.Outputs, filepath.Join(outDir, name))
			}
		}
		sort.Strings(result.Outputs)

	case SplitByRange:
		template := opts.NameTemplate
		if template == "" {
			template = "intervalo-%03d.pdf"
		}
		for i, r := range opts.Ranges {
			if err := ctx.Err(); err != nil {
				return SplitResult{}, err
			}
			name := SanitizeFilename(fmt.Sprintf(template, i+1))
			if name == "" {
				name = fmt.Sprintf("intervalo-%03d.pdf", i+1)
			}
			outFile := filepath.Join(outDir, name)
			if !opts.Overwrite {
				if _, err := os.Stat(outFile); err == nil {
					return SplitResult{}, fmt.Errorf("arquivo de saída %q já existe; use --overwrite para sobrescrever", outFile)
				}
			}
			if err := api.TrimFile(opts.Input, outFile, []string{r}, nil); err != nil {
				return SplitResult{}, fmt.Errorf("separar intervalo %q: %w", r, err)
			}
			result.Outputs = append(result.Outputs, outFile)
		}

	case SplitByRegex:
		if opts.Regex == nil {
			return SplitResult{}, fmt.Errorf("modo regex requer uma expressão regular")
		}

		pageTexts, err := ExtractPageTexts(opts.Input)
		if err != nil {
			return SplitResult{}, fmt.Errorf("extrair texto de %q: %w", opts.Input, err)
		}

		allEmpty := true
		for _, t := range pageTexts {
			if strings.TrimSpace(t) != "" {
				allEmpty = false
				break
			}
		}
		if allEmpty && len(pageTexts) > 0 {
			result.Warnings = append(result.Warnings, "nenhum texto foi extraído do PDF; ele pode ser um documento digitalizado (imagem sem camada de texto) e esta ferramenta não faz OCR")
		}

		matched := false
		for _, t := range pageTexts {
			if opts.Regex.MatchString(t) {
				matched = true
				break
			}
		}
		if !matched && len(pageTexts) > 0 {
			result.Warnings = append(result.Warnings, "a expressão regular não encontrou nenhum match; todas as páginas foram mantidas em um único arquivo")
		}

		groups := GroupPagesByRegex(pageTexts, opts.Regex, opts.NameTemplate)

		for _, g := range groups {
			if err := ctx.Err(); err != nil {
				return SplitResult{}, err
			}
			outFile := filepath.Join(outDir, g.Name+".pdf")
			if !opts.Overwrite {
				if _, err := os.Stat(outFile); err == nil {
					return SplitResult{}, fmt.Errorf("arquivo de saída %q já existe; use --overwrite para sobrescrever", outFile)
				}
			}
			rangeStr := fmt.Sprintf("%d-%d", g.Start, g.End)
			if err := api.TrimFile(opts.Input, outFile, []string{rangeStr}, nil); err != nil {
				return SplitResult{}, fmt.Errorf("separar grupo %q: %w", g.Name, err)
			}
			result.Outputs = append(result.Outputs, outFile)
		}

	default:
		return SplitResult{}, fmt.Errorf("modo de separação desconhecido: %q", opts.Mode)
	}

	return result, nil
}
