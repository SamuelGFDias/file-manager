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
// caminho do arquivo gravado e os IDs de manifestos PENDENTES removidos
// pela poda automática que Save dispara em seguida (ver comentário abaixo);
// normalmente vazio.
func Save(m Manifest) (path string, prunedPending []string, err error) {
	dir, err := Dir()
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, fmt.Errorf("erro ao criar diretório de histórico %q: %w", dir, err)
	}

	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}

	if m.ID == "" {
		id, err := generateID(dir, m.CreatedAt)
		if err != nil {
			return "", nil, err
		}
		m.ID = id
	}

	out, err := yaml.Marshal(&m)
	if err != nil {
		return "", nil, fmt.Errorf("erro ao codificar manifesto %q: %w", m.ID, err)
	}

	p := manifestPath(dir, m.ID)
	if err := os.WriteFile(p, out, 0o644); err != nil {
		return "", nil, fmt.Errorf("erro ao gravar manifesto em %q: %w", p, err)
	}

	// Poda best-effort de manifestos expirados (ver Prune/pruneDetailed):
	// mesmo princípio já usado no projeto para o próprio manifesto de
	// histórico e para o relatório de organize-pdf — uma tarefa de
	// manutenção secundária nunca pode fazer uma gravação que já aconteceu
	// de verdade parecer que falhou. O ERRO da poda é silenciosamente
	// ignorado aqui; a próxima chamada a Save tenta de novo.
	//
	// O resultado (quais PENDENTES foram removidos), ao contrário do erro,
	// NÃO é descartado: apagar um manifesto pendente tira, em silêncio, a
	// capacidade de desfazer aquela operação — é exatamente o tipo de
	// surpresa que este projeto evita. É responsabilidade do chamador (o
	// comando organize-pdf) informar isso ao usuário via Result.Details. Um
	// manifesto já desfeito removido pela mesma poda não entra em
	// prunedPending: já cumpriu sua função, não há capacidade nenhuma
	// sendo tirada.
	pending, _, _ := pruneDetailed(time.Now(), PruneUndoneAfter, PrunePendingAfter, false)

	return p, pending, nil
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

// Header são os metadados de um manifesto, sem a lista de Entries — é o
// que List() devolve. Ver o comentário de List() para o porquê de existir
// separado de Manifest.
type Header struct {
	ID        string
	Tool      string
	CreatedAt time.Time
	InputDir  string
	OutputDir string
	Action    Action
	UndoneAt  *time.Time
	// EntryCount é quantos arquivos a operação afetou (len(Manifest.Entries)
	// no momento da gravação) — o suficiente para exibição ("12 arquivos")
	// sem reter a lista inteira na memória depois que List() retorna.
	EntryCount int
}

// ListDisplayLimit é quantos cabeçalhos "undo --list" e as telas
// interativas de desfazer mostram por padrão (os mais recentes primeiro).
// Um histórico com centenas de operações vira uma tela ilegível — pior
// ainda num survey.Select, onde cada item extra é uma linha a mais para
// navegar às cegas. Quem precisa ver mais usa --all (linha de comando) ou a
// opção "Ver operações mais antigas" (telas interativas).
const ListDisplayLimit = 20

// List devolve os CABEÇALHOS de todos os manifestos gravados, mais
// recentes primeiro (por CreatedAt; ID como desempate). Devolve um slice
// vazio (não um erro) quando o diretório de histórico ainda não existe —
// nenhuma operação foi registrada ainda, o que é um estado válido e
// esperado (ex: instalação nova, ou nenhum organize-pdf real rodado até
// agora).
//
// Um manifesto individual ilegível (arquivo truncado por disco cheio no
// meio de uma gravação anterior, processo interrompido no momento errado,
// YAML corrompido por qualquer outro motivo) NÃO interrompe a listagem: é
// pulado e reportado em warnings — uma linha em português por arquivo
// problemático, citando o nome do arquivo e o motivo —, e os demais
// manifestos continuam aparecendo normalmente. err só é devolvido quando o
// PRÓPRIO diretório de histórico não pode ser lido (ex: sem permissão);
// nunca por causa de um arquivo individual dentro dele. Antes desta
// distinção, um único manifesto corrompido derrubava List() inteiro — e
// com ela o "undo" de TODAS as outras operações, inclusive as íntegras: o
// pior lugar possível para um ponto único de falha, já que "undo" é
// exatamente o recurso que existe para socorrer o usuário quando algo deu
// errado.
//
// Custo de parse vs. memória retida: cada arquivo ainda precisa ser
// decodificado por inteiro para contar EntryCount — não existe hoje um
// índice separado com só os metadados, então o TEMPO de List() continua
// proporcional ao tamanho total do histórico (isso é inevitável sem esse
// índice). O que muda é a MEMÓRIA RETIDA depois que a função retorna: o
// Manifest completo (incluindo o slice de Entries, potencialmente grande)
// de cada arquivo sai de escopo assim que o Header correspondente é
// montado, então o slice de Entries vira lixo para o GC ali mesmo, dentro
// do laço — o que fica retido por manifesto passa a ser O(1) (os campos do
// Header), nunca mais O(entradas). Se o histórico crescer a ponto do
// CUSTO DE PARSE (não mais a memória) incomodar, um índice separado — um
// arquivo pequeno só com os cabeçalhos, atualizado a cada Save/MarkUndone/
// Prune — é a evolução natural (ver AGENTS.md, não implementado agora).
func List() (headers []Header, warnings []string, err error) {
	dir, err := Dir()
	if err != nil {
		return nil, nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Header{}, nil, nil
		}
		return nil, nil, fmt.Errorf("erro ao listar diretório de histórico %q: %w", dir, err)
	}

	headers = make([]Header, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}

		id := strings.TrimSuffix(e.Name(), ".yaml")

		raw, readErr := os.ReadFile(filepath.Join(dir, e.Name()))
		if readErr != nil {
			warnings = append(warnings, fmt.Sprintf("erro ao ler manifesto de histórico %q: %v", e.Name(), readErr))
			continue
		}

		// m fica restrito a este bloco: uma vez que o Header abaixo é
		// montado, m (e o slice potencialmente grande de m.Entries) sai de
		// escopo e pode ser coletado pelo GC — ver o comentário de List()
		// sobre memória retida.
		var m Manifest
		if decErr := yaml.Unmarshal(raw, &m); decErr != nil {
			warnings = append(warnings, fmt.Sprintf("erro ao decodificar manifesto de histórico %q: %v", e.Name(), decErr))
			continue
		}

		headers = append(headers, Header{
			ID:         id,
			Tool:       m.Tool,
			CreatedAt:  m.CreatedAt,
			InputDir:   m.InputDir,
			OutputDir:  m.OutputDir,
			Action:     m.Action,
			UndoneAt:   m.UndoneAt,
			EntryCount: len(m.Entries),
		})
	}

	sort.SliceStable(headers, func(i, j int) bool {
		if !headers[i].CreatedAt.Equal(headers[j].CreatedAt) {
			return headers[i].CreatedAt.After(headers[j].CreatedAt)
		}
		return headers[i].ID > headers[j].ID
	})

	return headers, warnings, nil
}

// PruneUndoneAfter é o tempo mínimo que um manifesto JÁ DESFEITO continua no
// disco, contado a partir de UndoneAt, antes de se tornar elegível para
// remoção por Prune. Um manifesto desfeito já cumpriu sua função — o único
// motivo de mantê-lo por um tempo é permitir conferir o histórico recente,
// não desfazer de novo (que exigiria --force de qualquer forma).
const PruneUndoneAfter = 30 * 24 * time.Hour

// PrunePendingAfter é o tempo mínimo que um manifesto PENDENTE (nunca
// desfeito) continua no disco, contado a partir de CreatedAt, antes de se
// tornar elegível para remoção por Prune. Existe separado — e bem mais
// longo — de PruneUndoneAfter porque remover um pendente tira, de verdade,
// a capacidade de desfazer aquela operação; a justificativa aqui não é
// "já cumpriu a função" (como para os desfeitos), e sim que desfazer algo
// de 6 meses atrás deixou de ser realista: o destino provavelmente já foi
// reorganizado, movido ou apagado por fora do file-manager, e o "tamanho
// mudou" (SkipSizeChanged) tornaria o desfazer inútil de qualquer forma.
// Sem esta poda, o caso mais comum de uso — organizar e nunca desfazer —
// acumulava um manifesto pendente por execução, para sempre: só a poda dos
// já desfeitos (que é rara, poucos usuários chegam a desfazer) não
// resolvia nada para a maioria de quem usa a ferramenta.
const PrunePendingAfter = 180 * 24 * time.Hour

// pruneDetailed é a implementação real de Prune e PrunePlan: percorre os
// cabeçalhos do histórico (via List(), que já tolera manifesto ilegível —
// ver seu comentário) e separa os candidatos a poda em duas categorias,
// PENDENTES e JÁ DESFEITOS, porque o chamador (Save, ver seu comentário)
// precisa saber qual categoria perdeu manifestos para decidir se avisa o
// usuário. dryRun segue o mesmo idioma já usado por Undo (dryRun, force)
// neste pacote: true calcula o que SERIA removido sem tocar em nada no
// disco (usado por PrunePlan, para "undo --prune" poder pedir confirmação
// antes de apagar de verdade); false remove de fato (usado por Prune).
//
// Erro ao remover um manifesto específico interrompe a poda e devolve,
// junto do erro, o que já foi removido até ali (nunca deixa a chamadora
// sem saber o que já aconteceu) — mesmo padrão do Prune anterior.
func pruneDetailed(now time.Time, undoneAfter, pendingAfter time.Duration, dryRun bool) (removedPending, removedUndone []string, err error) {
	dir, err := Dir()
	if err != nil {
		return nil, nil, err
	}

	headers, _, err := List()
	if err != nil {
		return nil, nil, err
	}

	removedPending = make([]string, 0)
	removedUndone = make([]string, 0)

	for _, h := range headers {
		if h.UndoneAt != nil {
			if now.Sub(*h.UndoneAt) < undoneAfter {
				continue
			}
			if !dryRun {
				if err := os.Remove(manifestPath(dir, h.ID)); err != nil {
					return removedPending, removedUndone, fmt.Errorf("erro ao remover manifesto expirado %q: %w", h.ID, err)
				}
			}
			removedUndone = append(removedUndone, h.ID)
			continue
		}

		if now.Sub(h.CreatedAt) < pendingAfter {
			continue
		}
		if !dryRun {
			if err := os.Remove(manifestPath(dir, h.ID)); err != nil {
				return removedPending, removedUndone, fmt.Errorf("erro ao remover manifesto pendente expirado %q: %w", h.ID, err)
			}
		}
		removedPending = append(removedPending, h.ID)
	}

	return removedPending, removedUndone, nil
}

// Prune remove do disco os manifestos expirados, contado a partir de now:
// já desfeitos (UndoneAt != nil) há mais de undoneAfter, e PENDENTES
// (UndoneAt == nil, nunca desfeitos) há mais de pendingAfter desde
// CreatedAt — ver PruneUndoneAfter e PrunePendingAfter. Devolve os IDs
// efetivamente removidos, nas duas categorias combinadas (nunca nil; slice
// vazio quando nada foi removido). Nenhum manifesto pendente MAIS NOVO que
// pendingAfter é tocado, sob nenhuma condição — é exatamente ele que
// permite desfazer aquela operação mais tarde.
func Prune(now time.Time, undoneAfter, pendingAfter time.Duration) ([]string, error) {
	pending, undone, err := pruneDetailed(now, undoneAfter, pendingAfter, false)
	removed := make([]string, 0, len(pending)+len(undone))
	removed = append(removed, pending...)
	removed = append(removed, undone...)
	return removed, err
}

// PrunePlan calcula exatamente o que Prune(now, undoneAfter, pendingAfter)
// removeria, SEM TOCAR em nada no disco — o mesmo espírito de
// Undo(m, dryRun=true, force): existe para "undo --prune" poder mostrar e
// pedir confirmação sobre uma poda manual antes de apagar qualquer
// manifesto de verdade. Devolve os IDs pendentes e já desfeitos
// separadamente (ao contrário de Prune) porque só a perda de um pendente
// precisa ser destacada na confirmação — um já desfeito já cumpriu sua
// função.
func PrunePlan(now time.Time, undoneAfter, pendingAfter time.Duration) (removedPending, removedUndone []string, err error) {
	return pruneDetailed(now, undoneAfter, pendingAfter, true)
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
