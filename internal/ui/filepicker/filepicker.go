package filepicker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
)

// Entry é um item listável de um diretório.
type Entry struct {
	Name  string // nome base
	Path  string // caminho absoluto
	IsDir bool
}

// ErrCancelled é devolvido quando o usuário aborta a seleção.
var ErrCancelled = errors.New("filepicker: seleção cancelada")

// ListDir devolve as entradas de dir: subdiretórios primeiro (ordenados por nome),
// depois os arquivos que casem com exts (ordenados por nome).
// exts é uma lista de extensões com ponto e minúsculas, ex []string{".pdf"}.
// exts vazio ou nil = todos os arquivos.
// Diretórios e arquivos ocultos (nome começando com ".") são omitidos.
func ListDir(dir string, exts []string) ([]Entry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler diretório %s: %w", dir, err)
	}

	var dirs, files []Entry

	for _, entry := range entries {
		name := entry.Name()
		// Ignora entradas ocultas
		if strings.HasPrefix(name, ".") {
			continue
		}

		absPath, err := filepath.Abs(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("erro ao obter caminho absoluto de %s: %w", name, err)
		}

		e := Entry{
			Name:  name,
			Path:  absPath,
			IsDir: entry.IsDir(),
		}

		if entry.IsDir() {
			dirs = append(dirs, e)
		} else {
			// Filtro de extensão
			if len(exts) == 0 {
				// Sem filtro: inclui todos os arquivos
				files = append(files, e)
			} else {
				// Filtro case-insensitive
				ext := strings.ToLower(filepath.Ext(name))
				for _, filterExt := range exts {
					if ext == strings.ToLower(filterExt) {
						files = append(files, e)
						break
					}
				}
			}
		}
	}

	// Ordena diretórios alfabeticamente case-insensitive
	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})

	// Ordena arquivos alfabeticamente case-insensitive
	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	// Diretórios primeiro, depois arquivos
	result := append(dirs, files...)
	return result, nil
}

// PickFile navega a partir de start e devolve o caminho absoluto de UM arquivo escolhido.
func PickFile(start string, exts []string) (string, error) {
	cur, err := normalizePath(start)
	if err != nil {
		return "", err
	}

	for {
		entries, err := ListDir(cur, exts)
		if err != nil {
			return "", err
		}

		options := buildOptions(cur, entries, false)

		selected := ""
		promptErr := survey.AskOne(
			&survey.Select{
				Message: fmt.Sprintf("Navegue até o arquivo desejado\nDiretório atual: %s", cur),
				Options: options,
			},
			&selected,
		)

		if promptErr != nil {
			if errors.Is(promptErr, terminal.InterruptErr) {
				return "", ErrCancelled
			}
			return "", promptErr
		}

		if selected == "[ Cancelar ]" {
			return "", ErrCancelled
		}

		// Busca qual entry foi selecionada
		for _, e := range entries {
			display := e.Name
			if e.IsDir {
				display += "/"
			}
			if display == selected {
				if e.IsDir {
					cur = e.Path
					break
				}
				return e.Path, nil
			}
		}

		// Verifica se foi ".." (voltar)
		if selected == ".. (voltar)" {
			parent := filepath.Dir(cur)
			if parent != cur { // Não está na raiz
				cur = parent
			}
		}
	}
}

// PickFiles navega a partir de start e devolve vários arquivos do diretório atual (multi-seleção).
func PickFiles(start string, exts []string) ([]string, error) {
	cur, err := normalizePath(start)
	if err != nil {
		return nil, err
	}

	for {
		entries, err := ListDir(cur, exts)
		if err != nil {
			return nil, err
		}

		// Separar diretórios de arquivos
		var dirs []Entry
		var files []Entry
		for _, e := range entries {
			if e.IsDir {
				dirs = append(dirs, e)
			} else {
				files = append(files, e)
			}
		}

		// Primeira pergunta: navegação entre pastas
		navOptions := []string{}
		for _, d := range dirs {
			navOptions = append(navOptions, d.Name+"/")
		}

		// Adiciona opção de voltar se não está na raiz
		if filepath.Dir(cur) != cur {
			navOptions = append(navOptions, ".. (voltar)")
		}

		// Adiciona opção para escolher arquivos desta pasta
		navOptions = append(navOptions, "[ Escolher arquivos desta pasta ]")

		// Adiciona opção de cancelar
		navOptions = append(navOptions, "[ Cancelar ]")

		selectedNav := ""
		navPromptErr := survey.AskOne(
			&survey.Select{
				Message: fmt.Sprintf("Navegue ou selecione pasta\nDiretório atual: %s", cur),
				Options: navOptions,
			},
			&selectedNav,
		)

		if navPromptErr != nil {
			if errors.Is(navPromptErr, terminal.InterruptErr) {
				return nil, ErrCancelled
			}
			return nil, navPromptErr
		}

		if selectedNav == "[ Cancelar ]" {
			return nil, ErrCancelled
		}

		if selectedNav == "[ Escolher arquivos desta pasta ]" {
			// Segunda pergunta: multi-seleção de arquivos
			if len(files) == 0 {
				// Sem arquivos, mostra mensagem e volta ao menu
				fmt.Println("Nenhum arquivo disponível nesta pasta.")
				continue
			}

			fileOptions := []string{}
			for _, f := range files {
				fileOptions = append(fileOptions, f.Name)
			}

			selectedFiles := []string{}
			filesPromptErr := survey.AskOne(
				&survey.MultiSelect{
					Message: "Selecione os arquivos",
					Options: fileOptions,
				},
				&selectedFiles,
			)

			if filesPromptErr != nil {
				if errors.Is(filesPromptErr, terminal.InterruptErr) {
					return nil, ErrCancelled
				}
				return nil, filesPromptErr
			}

			// Converte nomes selecionados para caminhos absolutos
			var result []string
			for _, selected := range selectedFiles {
				for _, f := range files {
					if f.Name == selected {
						result = append(result, f.Path)
						break
					}
				}
			}

			return result, nil
		}

		// Navegação em diretórios
		if selectedNav == ".. (voltar)" {
			parent := filepath.Dir(cur)
			if parent != cur { // Não está na raiz
				cur = parent
			}
			continue
		}

		// Entra no diretório selecionado
		for _, d := range dirs {
			if d.Name+"/" == selectedNav {
				cur = d.Path
				break
			}
		}
	}
}

// PickDir navega a partir de start e devolve o caminho absoluto de um DIRETÓRIO escolhido.
func PickDir(start string) (string, error) {
	cur, err := normalizePath(start)
	if err != nil {
		return "", err
	}

	for {
		entries, err := ListDir(cur, nil)
		if err != nil {
			return "", err
		}

		options := []string{"[ Selecionar esta pasta ]"}

		// Adiciona subdiretórios
		for _, e := range entries {
			if e.IsDir {
				options = append(options, e.Name+"/")
			}
		}

		// Adiciona opção de voltar se não está na raiz
		if filepath.Dir(cur) != cur {
			options = append(options, ".. (voltar)")
		}

		// Adiciona opção de cancelar
		options = append(options, "[ Cancelar ]")

		selected := ""
		promptErr := survey.AskOne(
			&survey.Select{
				Message: fmt.Sprintf("Selecione um diretório\nDiretório atual: %s", cur),
				Options: options,
			},
			&selected,
		)

		if promptErr != nil {
			if errors.Is(promptErr, terminal.InterruptErr) {
				return "", ErrCancelled
			}
			return "", promptErr
		}

		if selected == "[ Cancelar ]" {
			return "", ErrCancelled
		}

		if selected == "[ Selecionar esta pasta ]" {
			return cur, nil
		}

		if selected == ".. (voltar)" {
			parent := filepath.Dir(cur)
			if parent != cur { // Não está na raiz
				cur = parent
			}
			continue
		}

		// Entra no diretório selecionado
		for _, e := range entries {
			if e.IsDir && e.Name+"/" == selected {
				cur = e.Path
				break
			}
		}
	}
}

// normalizePath converte start para um caminho absoluto.
// Se start vazio, usa o diretório de trabalho atual.
func normalizePath(start string) (string, error) {
	if start == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("erro ao obter diretório de trabalho: %w", err)
		}
		return wd, nil
	}

	abs, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("erro ao normalizar caminho %s: %w", start, err)
	}

	return abs, nil
}

// buildOptions constrói a lista de opções exibida para o usuário.
// isMultiSelect indica se é para PickFiles.
func buildOptions(cur string, entries []Entry, isMultiSelect bool) []string {
	options := []string{}

	for _, e := range entries {
		display := e.Name
		if e.IsDir {
			display += "/"
		}
		options = append(options, display)
	}

	// Adiciona opção de voltar se não está na raiz
	if filepath.Dir(cur) != cur {
		options = append(options, ".. (voltar)")
	}

	// Adiciona opção de cancelar
	options = append(options, "[ Cancelar ]")

	return options
}
