package pdfutil

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// MergeOptions descreve os parâmetros de uma operação de união de PDFs.
type MergeOptions struct {
	Inputs    []string // arquivos e/ou diretórios
	MaxDepth  int      // 0 = só a pasta informada; N = desce N níveis; -1 = ilimitado
	Output    string
	Sort      string // "name" (default) ou "mtime"
	Overwrite bool
}

// MergeResult descreve o resultado de uma operação de união de PDFs.
type MergeResult struct {
	Files     []string // arquivos efetivamente unidos, na ordem
	Output    string
	PageCount int
}

// ResolveInputs expande a lista de entradas (arquivos e/ou diretórios) em uma
// lista final e ordenada de arquivos PDF, sem duplicatas.
//
// Para diretórios, maxDepth controla a profundidade de varredura relativa à
// raiz informada: 0 não desce a subdiretórios, N>0 desce até N níveis, e -1
// varre sem limite.
func ResolveInputs(inputs []string, maxDepth int, sortBy string) ([]string, error) {
	seen := make(map[string]struct{})
	var result []string

	addFile := func(path string) error {
		abs, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("resolver caminho absoluto de %q: %w", path, err)
		}
		clean := filepath.Clean(abs)
		if _, ok := seen[clean]; ok {
			return nil
		}
		seen[clean] = struct{}{}
		result = append(result, path)
		return nil
	}

	for _, in := range inputs {
		info, err := os.Stat(in)
		if err != nil {
			return nil, fmt.Errorf("entrada %q não existe: %w", in, err)
		}

		if !info.IsDir() {
			if strings.EqualFold(filepath.Ext(in), ".pdf") {
				if err := addFile(in); err != nil {
					return nil, err
				}
			}
			continue
		}

		root := in
		walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path == root {
				return nil
			}

			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			depth := strings.Count(filepath.ToSlash(rel), "/") + 1

			if d.IsDir() {
				if maxDepth >= 0 && depth > maxDepth {
					return filepath.SkipDir
				}
				return nil
			}

			// depth aqui é a profundidade do arquivo; um arquivo diretamente
			// na raiz tem depth 1, o que corresponde a maxDepth 0.
			if maxDepth >= 0 && depth-1 > maxDepth {
				return nil
			}

			if strings.EqualFold(filepath.Ext(path), ".pdf") {
				return addFile(path)
			}
			return nil
		})
		if walkErr != nil {
			return nil, fmt.Errorf("percorrer diretório %q: %w", root, walkErr)
		}
	}

	if sortBy == "mtime" {
		sort.SliceStable(result, func(i, j int) bool {
			ii, errI := os.Stat(result[i])
			jj, errJ := os.Stat(result[j])
			if errI != nil || errJ != nil {
				return result[i] < result[j]
			}
			return ii.ModTime().Before(jj.ModTime())
		})
	} else {
		sort.Strings(result)
	}

	return result, nil
}

// Merge une os PDFs resolvidos a partir de opts.Inputs em um único arquivo de
// saída.
func Merge(ctx context.Context, opts MergeOptions) (MergeResult, error) {
	files, err := ResolveInputs(opts.Inputs, opts.MaxDepth, opts.Sort)
	if err != nil {
		return MergeResult{}, err
	}
	if len(files) == 0 {
		return MergeResult{}, fmt.Errorf("nenhum PDF encontrado nas entradas informadas")
	}

	if !opts.Overwrite {
		if _, err := os.Stat(opts.Output); err == nil {
			return MergeResult{}, fmt.Errorf("arquivo de saída %q já existe; use --overwrite para sobrescrever", opts.Output)
		}
	}

	if dir := filepath.Dir(opts.Output); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return MergeResult{}, fmt.Errorf("criar diretório de saída %q: %w", dir, err)
		}
	}

	if err := ctx.Err(); err != nil {
		return MergeResult{}, err
	}

	if err := api.MergeCreateFile(files, opts.Output, false, nil); err != nil {
		return MergeResult{}, fmt.Errorf("unir PDFs em %q: %w", opts.Output, err)
	}

	result := MergeResult{
		Files:  files,
		Output: opts.Output,
	}

	if count, err := api.PageCountFile(opts.Output); err == nil {
		result.PageCount = count
	}

	return result, nil
}
