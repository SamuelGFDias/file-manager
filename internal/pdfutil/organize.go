package pdfutil

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Level descreve um nível de pasta na hierarquia de organização, resolvido a
// partir de uma expressão regular aplicada ao texto extraído do PDF.
type Level struct {
	Label string
	Regex *regexp.Regexp
}

// Unmatched descreve por que um documento não pôde ser classificado.
type Unmatched struct {
	Level   string // rótulo do nível que falhou, ou "filename"
	Pattern string
}

// ResolveDestination calcula o caminho relativo de destino de um documento a
// partir do texto extraído dele, aplicando cada nível em ordem e, por fim, a
// expressão de nome de arquivo. Devolve unmatched != nil quando a
// classificação falha em algum ponto.
func ResolveDestination(text string, levels []Level, filenameRegex *regexp.Regexp) (relPath string, unmatched *Unmatched) {
	components := make([]string, 0, len(levels))

	for _, level := range levels {
		m := level.Regex.FindStringSubmatch(text)
		if m == nil {
			return "", &Unmatched{Level: level.Label, Pattern: level.Regex.String()}
		}
		value := m[0]
		if len(m) > 1 {
			value = m[1]
		}
		value = SanitizeFilename(value)
		if value == "" {
			return "", &Unmatched{Level: level.Label, Pattern: level.Regex.String()}
		}
		components = append(components, value)
	}

	var name string
	if filenameRegex != nil {
		m := filenameRegex.FindStringSubmatch(text)
		if m == nil {
			return "", &Unmatched{Level: "filename", Pattern: filenameRegex.String()}
		}
		value := m[0]
		if len(m) > 1 {
			value = m[1]
		}
		value = SanitizeFilename(value)
		if value == "" {
			return "", &Unmatched{Level: "filename", Pattern: filenameRegex.String()}
		}
		name = value
	}

	parts := append([]string{}, components...)
	if name != "" {
		parts = append(parts, name+".pdf")
	}
	result := filepath.Join(parts...)

	if result != "" && (filepath.IsAbs(result) || result == ".." || strings.HasPrefix(result, ".."+string(filepath.Separator))) {
		return "", &Unmatched{Level: "destino", Pattern: "caminho resultante inválido"}
	}

	return result, nil
}

// OrganizeOptions descreve os parâmetros de uma operação de organização de
// uma pasta de PDFs.
type OrganizeOptions struct {
	InputDir        string
	OutputDir       string
	Levels          []Level        // pode ser vazio => modo "somente renomear"
	FilenameRegex   *regexp.Regexp // se nil, mantém o nome original do arquivo
	Copy            bool           // true = copia (default do CLI), false = move
	UnclassifiedDir string         // default "sem-classificacao"
	DryRun          bool
	Sample          int // 0 = todos; N>0 = só os N primeiros (ordem alfabética)
	Overwrite       bool
}

// OrganizeEntry descreve o destino calculado (ou tentado) para um único
// arquivo.
type OrganizeEntry struct {
	Source    string
	Dest      string     // caminho relativo ao OutputDir
	Unmatched *Unmatched // nil quando classificado com sucesso
}

// OrganizeResult descreve o resultado de uma operação de organização.
type OrganizeResult struct {
	Organized    []OrganizeEntry
	Unclassified []OrganizeEntry
	DryRun       bool
	Total        int
}

// Summary devolve um resumo textual curto do resultado da organização.
func (r OrganizeResult) Summary() string {
	prefix := ""
	if r.DryRun {
		prefix = "[simulação] "
	}
	return fmt.Sprintf("%s%d de %d arquivos organizados, %d em sem-classificacao", prefix, len(r.Organized), r.Total, len(r.Unclassified))
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("abrir %q: %w", src, err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("criar %q: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("copiar %q para %q: %w", src, dst, err)
	}
	return out.Close()
}

func moveOrCopyFile(src, dst string, copy bool) error {
	if dir := filepath.Dir(dst); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("criar diretório de destino %q: %w", dir, err)
		}
	}

	if copy {
		return copyFile(src, dst)
	}

	if err := os.Rename(src, dst); err != nil {
		// Rename falha entre sistemas de arquivos diferentes (ex: pendrive
		// para disco); cai para copiar + remover.
		if cErr := copyFile(src, dst); cErr != nil {
			return cErr
		}
		return os.Remove(src)
	}
	return nil
}

// Organize classifica e move/copia os PDFs de InputDir para OutputDir de
// acordo com Levels e FilenameRegex.
func Organize(ctx context.Context, opts OrganizeOptions) (OrganizeResult, error) {
	if err := ctx.Err(); err != nil {
		return OrganizeResult{}, err
	}

	if opts.InputDir == "" {
		return OrganizeResult{}, fmt.Errorf("diretório de entrada não informado")
	}

	dirEntries, err := os.ReadDir(opts.InputDir)
	if err != nil {
		return OrganizeResult{}, fmt.Errorf("ler diretório de entrada %q: %w", opts.InputDir, err)
	}

	var files []string
	for _, e := range dirEntries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".pdf") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	if opts.Sample > 0 && opts.Sample < len(files) {
		files = files[:opts.Sample]
	}

	unclassifiedDir := opts.UnclassifiedDir
	if unclassifiedDir == "" {
		unclassifiedDir = "sem-classificacao"
	}

	result := OrganizeResult{DryRun: opts.DryRun, Total: len(files)}

	for _, name := range files {
		if err := ctx.Err(); err != nil {
			return OrganizeResult{}, err
		}

		srcPath := filepath.Join(opts.InputDir, name)

		var unmatched *Unmatched
		var dest string

		text, textErr := ExtractText(srcPath)
		if textErr != nil {
			unmatched = &Unmatched{Level: "texto", Pattern: "falha ao extrair texto"}
		} else {
			relPath, um := ResolveDestination(text, opts.Levels, opts.FilenameRegex)
			if um != nil {
				unmatched = um
			} else {
				if opts.FilenameRegex == nil {
					relPath = filepath.Join(relPath, name)
				}
				dest = relPath
			}
		}

		if unmatched != nil {
			dest = filepath.Join(unclassifiedDir, name)
		}

		entry := OrganizeEntry{Source: srcPath, Dest: dest, Unmatched: unmatched}

		if !opts.DryRun {
			destAbs := filepath.Join(opts.OutputDir, dest)

			if !opts.Overwrite {
				if _, statErr := os.Stat(destAbs); statErr == nil && unmatched == nil {
					// Colisão no destino classificado: reclassifica em vez
					// de abortar a execução inteira.
					unmatched = &Unmatched{Level: "destino", Pattern: "destino já existe"}
					dest = filepath.Join(unclassifiedDir, name)
					entry.Dest = dest
					entry.Unmatched = unmatched
					destAbs = filepath.Join(opts.OutputDir, dest)
				}
			}

			proceed := true
			if !opts.Overwrite {
				if _, statErr := os.Stat(destAbs); statErr == nil {
					// Colisão persiste mesmo após reclassificar (ex: dentro
					// de sem-classificacao). Mantém o registro, mas não
					// sobrescreve o arquivo existente.
					proceed = false
				}
			}

			if proceed {
				if err := moveOrCopyFile(srcPath, destAbs, opts.Copy); err != nil {
					return OrganizeResult{}, fmt.Errorf("organizar %q: %w", srcPath, err)
				}
			}
		}

		if unmatched != nil {
			result.Unclassified = append(result.Unclassified, entry)
		} else {
			result.Organized = append(result.Organized, entry)
		}
	}

	return result, nil
}
