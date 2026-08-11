package filepicker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

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

// lastDirMu protege lastDir, a memória (em nível de pacote) do último
// diretório em que uma seleção de arquivo ou pasta foi concluída com
// sucesso. Existe para que, sem um start explícito, um seletor não abra
// sempre no diretório de trabalho do processo — que na prática costuma ser
// a pasta onde o executável foi deixado, quase nunca a pasta que o usuário
// quer navegar.
var (
	lastDirMu sync.Mutex
	lastDir   string
)

// LastDir devolve o último diretório em que uma seleção (de arquivo ou
// pasta) foi concluída com sucesso por PickDir, PickDirWithPrompt, PickFile
// ou PickFileWithPrompt, ou "" se nenhuma seleção foi concluída ainda.
func LastDir() string {
	lastDirMu.Lock()
	defer lastDirMu.Unlock()
	return lastDir
}

// ResetLastDir limpa a memória de último diretório. Existe principalmente
// para os testes isolarem estado entre casos — sem isso, um teste
// contaminaria o próximo.
func ResetLastDir() {
	lastDirMu.Lock()
	defer lastDirMu.Unlock()
	lastDir = ""
}

// setLastDir registra dir como o último diretório em que uma seleção foi
// concluída com sucesso. Seguro para chamada concorrente.
func setLastDir(dir string) {
	lastDirMu.Lock()
	defer lastDirMu.Unlock()
	lastDir = dir
}

// resolveStart decide o diretório de partida efetivo de uma navegação.
//
// Um start explícito e não-vazio SEMPRE vence: a memória de último
// diretório (LastDir) só entra em jogo quando o chamador passa "". Isso é
// proposital — a memória não deve sobrescrever silenciosamente um start
// explícito, o que tornaria o comportamento imprevisível para quem chama
// (ex: um fluxo que encadeia origem → destino → amostra passando o
// diretório anterior como start precisa que esse valor seja respeitado à
// risca). Com start vazio e sem memória ainda registrada, cai no diretório
// de trabalho atual — o comportamento histórico.
func resolveStart(start string) (string, error) {
	if start != "" {
		return normalizePath(start)
	}
	if ld := LastDir(); ld != "" {
		return normalizePath(ld)
	}
	return normalizePath("")
}

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
// Usa uma mensagem genérica; para um rótulo específico do contexto de uso, veja PickFileWithPrompt.
func PickFile(start string, exts []string) (string, error) {
	return PickFileWithPrompt(start, "Navegue até o arquivo desejado", exts)
}

// PickFileWithPrompt navega a partir de start e devolve o caminho absoluto de
// UM arquivo escolhido, exibindo prompt como mensagem (seguida do diretório
// atual) — útil para deixar claro ao usuário o que está sendo escolhido
// (origem, destino, amostra etc.) quando um fluxo encadeia várias seleções.
func PickFileWithPrompt(start, prompt string, exts []string) (string, error) {
	cur, err := resolveStart(start)
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
				Message: fmt.Sprintf("%s\nDiretório atual: %s", prompt, cur),
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
				setLastDir(cur)
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
// Usa uma mensagem genérica; para um rótulo específico do contexto de uso, veja PickDirWithPrompt.
func PickDir(start string) (string, error) {
	return PickDirWithPrompt(start, "Selecione um diretório")
}

// PickDirWithPrompt navega a partir de start e devolve o caminho absoluto de
// um DIRETÓRIO escolhido, exibindo prompt como mensagem (seguida do
// diretório atual) — útil para deixar claro ao usuário o que está sendo
// escolhido (origem, destino etc.) quando um fluxo encadeia várias seleções.
func PickDirWithPrompt(start, prompt string) (string, error) {
	cur, err := resolveStart(start)
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
				Message: fmt.Sprintf("%s\nDiretório atual: %s", prompt, cur),
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
			setLastDir(cur)
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
