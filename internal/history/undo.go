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

// Outcome devolve a linha de resumo de r, SEM NENHUM rótulo de
// prévia/simulação — quem decide se (e como) marcar um resultado como uma
// prévia é BuildUndoReport, nunca este método. Existe separado de Summary
// porque r.DryRun reflete apenas COMO Undo foi chamado internamente (o
// plano de confirmação sempre roda com dryRun=true, pedido pelo usuário ou
// não) — usar Summary() (que olha r.DryRun) para reportar o resultado de
// uma execução real que não restaurou nada, só porque o PLANO usado para
// decidir isso foi computado em modo simulado, é exatamente o defeito que
// fez a palavra "simulação" aparecer numa execução real.
//
// Distingue três casos quando nada foi restaurado:
//   - manifesto sem nenhuma entrada (r.Skipped também vazio);
//   - todas as entradas já estavam ausentes do destino ("nada a fazer":
//     não havia mesmo o que desfazer, alguém já cuidou disso por fora);
//   - havia arquivos no destino, mas cada um foi deliberadamente
//     preservado por algum motivo de segurança (tamanho mudou, origem
//     ocupada) — bem diferente de "nada a fazer": a operação decidiu não
//     mexer, por segurança, e o texto precisa deixar isso claro.
func (r UndoResult) Outcome() string {
	if len(r.Restored) > 0 {
		return fmt.Sprintf("%d arquivos restaurados, %d pulados", len(r.Restored), len(r.Skipped))
	}
	if len(r.Skipped) == 0 {
		return "nada a desfazer: o manifesto não tem nenhum arquivo registrado"
	}
	if allSkippedBecauseMissing(r.Skipped) {
		return "nada a fazer: todos os arquivos já estavam ausentes do destino"
	}
	return fmt.Sprintf(
		"nenhum arquivo foi restaurado: os %d arquivos encontrados no destino foram preservados por segurança (motivos acima)",
		len(r.Skipped),
	)
}

// allSkippedBecauseMissing indica se TODAS as entradas puladas o foram por
// já estarem ausentes do destino (SkipMissing) — o único caso em que
// "nada a fazer" é a descrição correta. Uma única entrada pulada por outro
// motivo (tamanho mudou, origem ocupada) já basta para que o resultado não
// seja "nada a fazer", e sim "preservado por segurança".
func allSkippedBecauseMissing(skipped []SkippedEntry) bool {
	for _, s := range skipped {
		if s.Reason != SkipMissing {
			return false
		}
	}
	return true
}

// Summary devolve a linha de resumo de r com o rótulo "[simulação] " à
// frente quando r.DryRun é true. Usada SÓ para reportar uma prévia pedida
// explicitamente pelo usuário (--dry-run) — para qualquer outro contexto
// (incluindo o plano interno calculado para dimensionar a confirmação de
// uma execução real), use Outcome() diretamente; ver BuildUndoReport.
func (r UndoResult) Summary() string {
	prefix := ""
	if r.DryRun {
		prefix = "[simulação] "
	}
	return prefix + r.Outcome()
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

// UndoReport é o texto exato a imprimir para uma chamada do comando
// "undo" (ou da tela equivalente), já decidido por BuildUndoReport —
// nenhum chamador deve montar essas linhas à mão, que foi exatamente o
// que deixou passar tanto o rótulo "[simulação]" indevido numa execução
// real quanto o resumo impresso duas vezes.
type UndoReport struct {
	// Skipped é uma linha por entrada pulada, para ser impressa ANTES de
	// Summary — quem lê vê primeiro o motivo de cada uma, só depois o
	// resultado geral que esses motivos explicam.
	Skipped []string
	// Summary é a última linha: o resultado geral da operação.
	Summary string
}

// Lines devolve o relatório inteiro, já na ordem de impressão (Skipped,
// depois Summary).
func (r UndoReport) Lines() []string {
	lines := make([]string, 0, len(r.Skipped)+1)
	lines = append(lines, r.Skipped...)
	lines = append(lines, r.Summary)
	return lines
}

// BuildUndoReport decide exatamente o que mostrar para uma chamada do
// comando "undo" (ou da tela internal/ui/undo): a única função, pura e
// sem I/O, que os dois chamadores usam — não podem divergir sobre o que
// "desfazer" significa nem sobre como reportar isso.
//
//   - preview é sempre o plano calculado com Undo(m, true, force): existe
//     de qualquer forma, porque é preciso para dimensionar a pergunta de
//     confirmação antes de tocar em qualquer arquivo.
//   - previewRequested é true SÓ quando o usuário passou --dry-run
//     explicitamente. Controla se Summary carrega o rótulo "[simulação]"
//     — e NUNCA deve ser derivado de preview.DryRun, que é sempre true
//     internamente (o plano roda em modo simulado tenha o usuário pedido
//     --dry-run ou não). Foi exatamente essa confusão — usar o campo
//     DryRun do plano interno para decidir o rótulo — que fazia
//     "[simulação]" aparecer numa execução real pedida sem --dry-run.
//   - final é o resultado da execução real (Undo(m, false, force)), ou
//     nil quando nada foi executado de verdade — seja porque
//     previewRequested é true (o usuário só queria ver o plano), seja
//     porque preview.Restored já estava vazio e o comando encerrou antes
//     de sequer perguntar confirmação.
//
// Consolidar a decisão aqui — e cada chamador imprimir exatamente UM
// UndoReport por invocação, nunca preview e final separadamente — é o que
// impede o resumo de ser composto (e mostrado) duas vezes.
func BuildUndoReport(preview UndoResult, previewRequested bool, final *UndoResult) UndoReport {
	if previewRequested {
		return UndoReport{Summary: preview.Summary(), Skipped: preview.SkippedLines()}
	}
	if final != nil {
		return UndoReport{Summary: final.Outcome(), Skipped: final.SkippedLines()}
	}
	return UndoReport{Summary: preview.Outcome(), Skipped: preview.SkippedLines()}
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
