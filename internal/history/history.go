// Package history grava e lê o manifesto de operações reais executadas por
// ferramentas do CLI que suportam desfazer (hoje, só organize-pdf). Cada
// manifesto é um arquivo YAML com a lista exata de arquivos copiados ou
// movidos — o suficiente para o comando "undo" (ver Undo, em undo.go, mesmo
// pacote) reverter a operação sem tocar em nada fora do que foi de fato
// registrado.
//
// Decisão de arquitetura: este pacote não importa internal/pdfutil nem é
// importado por ele. internal/pdfutil não pode depender de configuração do
// usuário (misturaria lógica pura de organização com gerenciamento de
// diretórios de configuração) — a gravação do manifesto é injetada de fora,
// via o campo Recorder de pdfutil.OrganizeOptions, e é internal/tools/
// organizepdf (o comando, não o domínio) quem faz a ponte entre
// pdfutil.RecordedEntry e history.Entry.
//
// Este pacote também não importa internal/config: em vez disso, repete o
// mesmo padrão usado lá (variável de pacote userConfigDir, substituível em
// teste) para resolver o diretório de configuração do usuário. É uma
// duplicação pequena e deliberada — evita uma dependência de pacote só para
// reaproveitar uma função de uma linha, e mantém os dois pacotes
// simetricamente testáveis, cada um isolado do outro.
//
// A lógica de execução do desfazer (verificação de tamanho, "não sobrescreva
// a origem", remoção de diretórios vazios) fica em undo.go, dentro deste
// mesmo pacote: é regra de negócio pura, tão testável quanto a gravação do
// manifesto, e mantê-la aqui evita um pacote extra só para não colidir com o
// nome do pacote de UI correspondente (internal/ui/undo).
package history

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// appName é o mesmo nome de subdiretório usado por internal/config
// (config.AppName) — duplicado aqui de propósito (ver comentário do
// pacote): evita uma dependência só por causa de uma constante.
const appName = "file-manager"

// userConfigDir resolve o diretório de configuração do usuário. É uma
// variável (e não uma chamada direta a os.UserConfigDir) para permitir que
// os testes a substituam por um diretório temporário — mesmo padrão usado
// em internal/config.
var userConfigDir = os.UserConfigDir

// Action identifica se uma entrada do manifesto foi copiada ou movida.
type Action string

const (
	// ActionCopy indica que as entradas do manifesto foram copiadas: o
	// original em Source nunca foi tocado, e desfazer significa apagar o
	// que foi criado em Dest.
	ActionCopy Action = "copy"
	// ActionMove indica que as entradas do manifesto foram movidas: Source
	// deixou de existir, e desfazer significa devolver o arquivo de Dest
	// para Source.
	ActionMove Action = "move"
)

// Entry descreve um único arquivo efetivamente copiado ou movido por uma
// operação registrada.
type Entry struct {
	// Source é o caminho absoluto de origem, no momento da operação
	// original.
	Source string `yaml:"source"`
	// Dest é o caminho absoluto de destino, criado pela operação original.
	Dest string `yaml:"dest"`
	// Size é o tamanho do arquivo em Dest, lido logo após a operação —
	// usado por Undo para detectar se o arquivo foi substituído ou editado
	// depois, antes de decidir apagá-lo ou movê-lo de volta.
	Size int64 `yaml:"size"`
}

// Manifest descreve uma execução real (nunca uma simulação) de uma
// ferramenta que suporta desfazer.
type Manifest struct {
	// ID é o identificador legível do manifesto, ex "20260811-164530", e
	// também o nome do arquivo (sem a extensão .yaml) em Dir().
	ID string `yaml:"id"`
	// Tool identifica a ferramenta que gravou o manifesto, ex
	// "organize-pdf".
	Tool string `yaml:"tool"`
	// CreatedAt é o instante em que a operação original foi executada.
	CreatedAt time.Time `yaml:"created_at"`
	// InputDir e OutputDir são os caminhos absolutos usados na operação
	// original — guardados para exibição em "undo --list" e na tela
	// interativa; a reversão em si usa só Entries.
	InputDir  string `yaml:"input_dir"`
	OutputDir string `yaml:"output_dir"`
	// Action indica se a operação original copiou ou moveu os arquivos.
	Action Action `yaml:"action"`
	// Entries é a lista de todos os arquivos efetivamente copiados ou
	// movidos, incluindo os que foram para a pasta de não-classificação —
	// eles também precisam poder voltar.
	Entries []Entry `yaml:"entries"`
	// UndoneAt é preenchido por MarkUndone quando o manifesto já foi
	// desfeito; nil enquanto pendente.
	UndoneAt *time.Time `yaml:"undone_at,omitempty"`
}

// Dir devolve o diretório onde os manifestos são gravados:
// <diretório de configuração do usuário>/file-manager/history.
func Dir() (string, error) {
	dir, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("erro ao resolver diretório de configuração do usuário: %w", err)
	}
	return filepath.Join(dir, appName, "history"), nil
}

// manifestPath devolve o caminho completo do arquivo de manifesto de um ID,
// dentro de dir (o valor já resolvido por Dir()).
func manifestPath(dir, id string) string {
	return filepath.Join(dir, id+".yaml")
}

// generateID monta um ID legível "AAAAMMDD-HHMMSS" a partir de when (hora
// local), acrescentando um sufixo numérico ("-2", "-3", ...) enquanto já
// existir um manifesto com esse ID em dir — duas operações registradas no
// mesmo segundo não podem se sobrescrever.
func generateID(dir string, when time.Time) (string, error) {
	base := when.Local().Format("20060102-150405")

	candidate := base
	for suffix := 2; ; suffix++ {
		_, err := os.Stat(manifestPath(dir, candidate))
		if os.IsNotExist(err) {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("erro ao verificar manifesto existente %q: %w", candidate, err)
		}
		candidate = fmt.Sprintf("%s-%d", base, suffix)
	}
}

// Save grava m em disco, gerando m.ID automaticamente quando está vazio (a
// partir de m.CreatedAt, ou do horário atual se CreatedAt também for zero) e
// evitando colisão com um manifesto já existente (ver generateID). Devolve o
// caminho do arquivo gravado.
func Save(m Manifest) (path string, err error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("erro ao criar diretório de histórico %q: %w", dir, err)
	}

	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}

	if m.ID == "" {
		id, err := generateID(dir, m.CreatedAt)
		if err != nil {
			return "", err
		}
		m.ID = id
	}

	out, err := yaml.Marshal(&m)
	if err != nil {
		return "", fmt.Errorf("erro ao codificar manifesto %q: %w", m.ID, err)
	}

	p := manifestPath(dir, m.ID)
	if err := os.WriteFile(p, out, 0o644); err != nil {
		return "", fmt.Errorf("erro ao gravar manifesto em %q: %w", p, err)
	}

	return p, nil
}

// Load carrega o manifesto de ID informado.
func Load(id string) (Manifest, error) {
	dir, err := Dir()
	if err != nil {
		return Manifest{}, err
	}

	raw, err := os.ReadFile(manifestPath(dir, id))
	if err != nil {
		if os.IsNotExist(err) {
			return Manifest{}, fmt.Errorf("operação %q não encontrada: %w", id, err)
		}
		return Manifest{}, fmt.Errorf("erro ao ler manifesto %q: %w", id, err)
	}

	var m Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return Manifest{}, fmt.Errorf("erro ao decodificar manifesto %q: %w", id, err)
	}

	return m, nil
}

// List devolve todos os manifestos gravados, mais recentes primeiro (por
// CreatedAt; ID como desempate). Devolve um slice vazio (não um erro) quando
// o diretório de histórico ainda não existe — nenhuma operação foi
// registrada ainda, o que é um estado válido e esperado (ex: instalação
// nova, ou nenhum organize-pdf real rodado até agora).
func List() ([]Manifest, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Manifest{}, nil
		}
		return nil, fmt.Errorf("erro ao listar diretório de histórico %q: %w", dir, err)
	}

	manifests := make([]Manifest, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".yaml")
		m, err := Load(id)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, m)
	}

	sort.SliceStable(manifests, func(i, j int) bool {
		if !manifests[i].CreatedAt.Equal(manifests[j].CreatedAt) {
			return manifests[i].CreatedAt.After(manifests[j].CreatedAt)
		}
		return manifests[i].ID > manifests[j].ID
	})

	return manifests, nil
}

// MarkUndone carrega o manifesto de ID informado, grava UndoneAt = when e
// persiste de volta no mesmo arquivo (o ID não muda).
func MarkUndone(id string, when time.Time) error {
	m, err := Load(id)
	if err != nil {
		return err
	}

	w := when
	m.UndoneAt = &w

	dir, err := Dir()
	if err != nil {
		return err
	}

	out, err := yaml.Marshal(&m)
	if err != nil {
		return fmt.Errorf("erro ao codificar manifesto %q: %w", id, err)
	}

	p := manifestPath(dir, id)
	if err := os.WriteFile(p, out, 0o644); err != nil {
		return fmt.Errorf("erro ao gravar manifesto %q: %w", id, err)
	}

	return nil
}
