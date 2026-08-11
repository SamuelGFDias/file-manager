package history

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// SkipReason explica, em português, por que uma entrada do manifesto não foi
// desfeita.
type SkipReason string

const (
	// SkipMissing indica que o arquivo em Dest já não existe mais — alguém
	// já apagou ou moveu manualmente.
	SkipMissing SkipReason = "já removido do destino"
	// SkipSizeChanged indica que o arquivo em Dest existe, mas seu tamanho
	// não bate mais com o registrado no manifesto — pode ter sido
	// substituído ou editado depois da organização original. A verificação
	// é por TAMANHO, não por conteúdo (ex: hash): comparar conteúdo seria
	// mais preciso, mas exigiria ler cada arquivo por inteiro só para
	// desfazer, custo que não compensa num lote com centenas de PDFs.
	SkipSizeChanged SkipReason = "tamanho do arquivo mudou desde a organização (pode ter sido editado depois); não apagado/movido por segurança"
	// SkipSourceExists indica que, ao tentar mover um arquivo de volta,
	// Entry.Source já está ocupado por outro arquivo — a entrada é pulada
	// em vez de sobrescrever.
	SkipSourceExists SkipReason = "já existe um arquivo na origem; não sobrescrito"
)

// SkippedEntry associa uma Entry do manifesto ao motivo pelo qual Undo não a
// desfez.
type SkippedEntry struct {
	Entry  Entry
	Reason SkipReason
}

// UndoResult é o resultado de uma chamada a Undo.
type UndoResult struct {
	Manifest Manifest
	DryRun   bool
	Restored []Entry
	Skipped  []SkippedEntry
}

// MaxSkippedDetails limita quantas linhas de SkippedLines são detalhadas
// antes de resumir o restante em "... e mais N" — despejar centenas de
// linhas numa tela de terminal não ajuda ninguém.
const MaxSkippedDetails = 10

// Summary devolve um resumo textual curto do resultado, ex: "12 arquivos
// restaurados, 2 pulados".
func (r UndoResult) Summary() string {
	prefix := ""
	if r.DryRun {
		prefix = "[simulação] "
	}
	return fmt.Sprintf("%s%d arquivos restaurados, %d pulados", prefix, len(r.Restored), len(r.Skipped))
}

// SkippedLines devolve uma linha por entrada pulada (caminho de destino e
// motivo), limitada a MaxSkippedDetails, com uma última linha "... e mais N"
// quando o total excede o limite.
func (r UndoResult) SkippedLines() []string {
	if len(r.Skipped) == 0 {
		return nil
	}

	lines := make([]string, 0, MaxSkippedDetails+1)
	for i, s := range r.Skipped {
		if i >= MaxSkippedDetails {
			lines = append(lines, fmt.Sprintf("... e mais %d", len(r.Skipped)-MaxSkippedDetails))
			break
		}
		lines = append(lines, fmt.Sprintf("%s: %s", s.Entry.Dest, s.Reason))
	}
	return lines
}

// ErrAlreadyUndone é devolvido por Undo quando o manifesto já tem UndoneAt
// preenchido e force é false.
var ErrAlreadyUndone = errors.New("esta operação já foi desfeita anteriormente")

// Undo desfaz m: para Action == ActionMove, tenta mover cada entrada de
// volta de Entry.Dest para Entry.Source; para Action == ActionCopy, apaga o
// arquivo em Entry.Dest (Entry.Source nunca é lido nem tocado). dryRun
// calcula exatamente o que seria feito sem tocar em nada no disco — mesmo
// função usada para a simulação e para a execução real, para que as duas
// nunca possam divergir. force permite desfazer um manifesto que já tem
// UndoneAt preenchido; sem force, devolve ErrAlreadyUndone antes de tocar em
// qualquer arquivo.
//
// Regras de segurança, o núcleo desta função:
//   - nunca toca em um arquivo fora de m.Entries — nenhum outro arquivo da
//     pasta de destino é sequer olhado;
//   - antes de tocar em Entry.Dest, verifica se ele ainda existe e se seu
//     tamanho bate com Entry.Size; um tamanho diferente pula a entrada (ver
//     SkipSizeChanged) em vez de apagá-la ou sobrescrevê-la;
//   - ao mover de volta (Action == ActionMove), se Entry.Source já existir,
//     a entrada é pulada sem sobrescrever (ver SkipSourceExists);
//   - diretórios que ficaram vazios no destino, depois do desfazer, são
//     removidos subindo de Entry.Dest até (sem incluir) m.OutputDir — nunca
//     com remoção recursiva: um diretório com qualquer arquivo dentro
//     (esperado ou não) é preservado.
func Undo(m Manifest, dryRun bool, force bool) (UndoResult, error) {
	if m.UndoneAt != nil && !force {
		return UndoResult{}, ErrAlreadyUndone
	}

	result := UndoResult{Manifest: m, DryRun: dryRun}

	for _, entry := range m.Entries {
		info, err := os.Stat(entry.Dest)
		if err != nil {
			if os.IsNotExist(err) {
				result.Skipped = append(result.Skipped, SkippedEntry{Entry: entry, Reason: SkipMissing})
				continue
			}
			return UndoResult{}, fmt.Errorf("erro ao verificar %q: %w", entry.Dest, err)
		}

		if info.Size() != entry.Size {
			result.Skipped = append(result.Skipped, SkippedEntry{Entry: entry, Reason: SkipSizeChanged})
			continue
		}

		if m.Action == ActionMove {
			if _, err := os.Stat(entry.Source); err == nil {
				result.Skipped = append(result.Skipped, SkippedEntry{Entry: entry, Reason: SkipSourceExists})
				continue
			} else if !os.IsNotExist(err) {
				return UndoResult{}, fmt.Errorf("erro ao verificar %q: %w", entry.Source, err)
			}
		}

		if !dryRun {
			switch m.Action {
			case ActionCopy:
				if err := os.Remove(entry.Dest); err != nil {
					return UndoResult{}, fmt.Errorf("erro ao remover %q: %w", entry.Dest, err)
				}
			case ActionMove:
				if err := moveBack(entry.Dest, entry.Source); err != nil {
					return UndoResult{}, fmt.Errorf("erro ao mover %q de volta para %q: %w", entry.Dest, entry.Source, err)
				}
			default:
				return UndoResult{}, fmt.Errorf("ação desconhecida no manifesto: %q", m.Action)
			}
		}

		result.Restored = append(result.Restored, entry)
	}

	if !dryRun && len(result.Restored) > 0 {
		removeEmptyDirsUpward(m.OutputDir, result.Restored)
	}

	return result, nil
}

// moveBack move src (Entry.Dest) para dst (Entry.Source), criando o
// diretório de dst se necessário. Tenta os.Rename primeiro; se falhar (ex:
// src e dst em sistemas de arquivos diferentes, caso comum quando o destino
// da organização é um pendrive ou disco de rede), cai para copiar + remover
// — mesma estratégia usada por pdfutil.moveOrCopyFile, duplicada aqui de
// propósito para não acoplar internal/history a internal/pdfutil.
func moveBack(src, dst string) error {
	if dir := filepath.Dir(dst); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("erro ao criar diretório de destino %q: %w", dir, err)
		}
	}

	if err := os.Rename(src, dst); err != nil {
		if cErr := copyFileContents(src, dst); cErr != nil {
			return cErr
		}
		return os.Remove(src)
	}
	return nil
}

func copyFileContents(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("erro ao abrir %q: %w", src, err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("erro ao criar %q: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("erro ao copiar %q para %q: %w", src, dst, err)
	}
	return out.Close()
}

// removeEmptyDirsUpward remove, para cada entrada restaurada, os diretórios
// que ficaram vazios no destino, subindo a partir do diretório de Entry.Dest
// até (sem incluir) outputDir. Não usa nenhuma memória entre chamadas: como
// todos os arquivos já foram restaurados/removidos antes desta função ser
// chamada (nunca entrelaçado com o laço principal de Undo), reprocessar o
// mesmo diretório para entradas irmãs é seguro e barato — a segunda
// tentativa apenas encontra o diretório já removido (ou não vazio) e para.
func removeEmptyDirsUpward(outputDir string, restored []Entry) {
	outputDir = filepath.Clean(outputDir)

	for _, entry := range restored {
		dir := filepath.Clean(filepath.Dir(entry.Dest))
		for dir != outputDir && dir != "." && dir != string(filepath.Separator) {
			if !removeIfEmpty(dir) {
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
}

// removeIfEmpty remove dir e devolve true se, e somente se, dir existe e
// está vazio. Nunca recorre a remoção recursiva: um diretório com qualquer
// conteúdo (esperado ou não) é preservado, e a função devolve false.
func removeIfEmpty(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		return false
	}
	return os.Remove(dir) == nil
}
