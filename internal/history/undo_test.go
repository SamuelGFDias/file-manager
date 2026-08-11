package history

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("criar diretório de %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("escrever %q: %v", path, err)
	}
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("esperava que %q existisse: %v", path, err)
	}
}

func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("esperava que %q não existisse mais, err=%v", path, err)
	}
}

// TestUndoCopyRemovesOnlyRegisteredFiles cobre a regra central: desfazer uma
// cópia apaga exatamente os arquivos do manifesto e não toca em um arquivo
// extra colocado na mesma pasta de destino, mesmo que não esteja registrado.
func TestUndoCopyRemovesOnlyRegisteredFiles(t *testing.T) {
	outputDir := t.TempDir()
	registered := filepath.Join(outputDir, "a.pdf")
	extra := filepath.Join(outputDir, "b-nao-registrado.pdf")

	writeFile(t, registered, "conteudo-a")
	writeFile(t, extra, "conteudo-b")

	info, err := os.Stat(registered)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	m := Manifest{
		ID:        "teste-copy",
		Tool:      "organize-pdf",
		OutputDir: outputDir,
		Action:    ActionCopy,
		Entries: []Entry{
			{Source: "/nao/importa/a.pdf", Dest: registered, Size: info.Size()},
		},
	}

	result, err := Undo(m, false, false)
	if err != nil {
		t.Fatalf("Undo() erro inesperado: %v", err)
	}
	if len(result.Restored) != 1 {
		t.Fatalf("esperava 1 restaurado, obteve %d (skipped=%+v)", len(result.Restored), result.Skipped)
	}

	mustNotExist(t, registered)
	mustExist(t, extra) // arquivo fora do manifesto NUNCA é tocado
}

// TestUndoMoveRestoresToSource cobre a devolução de um arquivo movido para
// a origem original.
func TestUndoMoveRestoresToSource(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "origem")
	outputDir := filepath.Join(root, "destino")

	source := filepath.Join(sourceDir, "a.pdf")
	dest := filepath.Join(outputDir, "a.pdf")

	// Simula o estado pós-organização: o arquivo já está só em dest.
	writeFile(t, dest, "conteudo-a")

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	m := Manifest{
		ID:        "teste-move",
		Tool:      "organize-pdf",
		OutputDir: outputDir,
		Action:    ActionMove,
		Entries: []Entry{
			{Source: source, Dest: dest, Size: info.Size()},
		},
	}

	result, err := Undo(m, false, false)
	if err != nil {
		t.Fatalf("Undo() erro inesperado: %v", err)
	}
	if len(result.Restored) != 1 {
		t.Fatalf("esperava 1 restaurado, obteve %d (skipped=%+v)", len(result.Restored), result.Skipped)
	}

	mustExist(t, source)
	mustNotExist(t, dest)
}

// TestUndoSkipsSizeMismatch é a regra de segurança mais importante: um
// arquivo cujo tamanho não bate mais com o registrado é pulado, não
// apagado — o teste prova explicitamente que ele continua existindo depois.
func TestUndoSkipsSizeMismatch(t *testing.T) {
	outputDir := t.TempDir()
	dest := filepath.Join(outputDir, "a.pdf")
	writeFile(t, dest, "conteudo original")

	m := Manifest{
		ID:        "teste-tamanho",
		OutputDir: outputDir,
		Action:    ActionCopy,
		Entries: []Entry{
			// Size deliberadamente errado (o registrado é bem menor que o
			// conteúdo real gravado acima).
			{Source: "/nao/importa/a.pdf", Dest: dest, Size: 3},
		},
	}

	result, err := Undo(m, false, false)
	if err != nil {
		t.Fatalf("Undo() erro inesperado: %v", err)
	}
	if len(result.Restored) != 0 {
		t.Fatalf("esperava 0 restaurados (tamanho não bate), obteve %d", len(result.Restored))
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason != SkipSizeChanged {
		t.Fatalf("esperava 1 pulado com motivo %q, obteve %+v", SkipSizeChanged, result.Skipped)
	}

	mustExist(t, dest) // NUNCA apagado quando o tamanho mudou
}

// TestUndoSkipsMissingDest confirma que um arquivo já ausente no destino é
// pulado sem erro.
func TestUndoSkipsMissingDest(t *testing.T) {
	outputDir := t.TempDir()
	dest := filepath.Join(outputDir, "ja-sumiu.pdf")

	m := Manifest{
		ID:        "teste-ausente",
		OutputDir: outputDir,
		Action:    ActionCopy,
		Entries: []Entry{
			{Source: "/nao/importa/a.pdf", Dest: dest, Size: 10},
		},
	}

	result, err := Undo(m, false, false)
	if err != nil {
		t.Fatalf("Undo() erro inesperado: %v", err)
	}
	if len(result.Restored) != 0 {
		t.Fatalf("esperava 0 restaurados, obteve %d", len(result.Restored))
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason != SkipMissing {
		t.Fatalf("esperava 1 pulado com motivo %q, obteve %+v", SkipMissing, result.Skipped)
	}
}

// TestUndoMoveSkipsWhenSourceOccupied confirma que devolver um arquivo
// movido nunca sobrescreve algo que já ocupa a origem.
func TestUndoMoveSkipsWhenSourceOccupied(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "origem", "a.pdf")
	dest := filepath.Join(root, "destino", "a.pdf")

	writeFile(t, dest, "conteudo-destino")
	writeFile(t, source, "conteudo-que-ja-estava-na-origem")

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	m := Manifest{
		ID:        "teste-origem-ocupada",
		OutputDir: filepath.Dir(dest),
		Action:    ActionMove,
		Entries: []Entry{
			{Source: source, Dest: dest, Size: info.Size()},
		},
	}

	result, err := Undo(m, false, false)
	if err != nil {
		t.Fatalf("Undo() erro inesperado: %v", err)
	}
	if len(result.Restored) != 0 {
		t.Fatalf("esperava 0 restaurados, obteve %d", len(result.Restored))
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason != SkipSourceExists {
		t.Fatalf("esperava 1 pulado com motivo %q, obteve %+v", SkipSourceExists, result.Skipped)
	}

	// Nem a origem nem o destino podem ter sido tocados.
	got, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("ler origem: %v", err)
	}
	if string(got) != "conteudo-que-ja-estava-na-origem" {
		t.Fatalf("origem foi sobrescrita: conteúdo = %q", got)
	}
	mustExist(t, dest)
}

// TestUndoRemovesEmptyDirsButPreservesNonEmpty cobre a remoção de
// diretórios vazios subindo até OutputDir, e a preservação de um diretório
// que ainda contém um arquivo estranho (não registrado) depois do desfazer.
func TestUndoRemovesEmptyDirsButPreservesNonEmpty(t *testing.T) {
	outputDir := t.TempDir()

	emptyBranchFile := filepath.Join(outputDir, "fornecedorA", "filial1", "a.pdf")
	writeFile(t, emptyBranchFile, "conteudo-a")

	strayBranchFile := filepath.Join(outputDir, "fornecedorB", "filial1", "b.pdf")
	strayFile := filepath.Join(outputDir, "fornecedorB", "filial1", "estranho.txt")
	writeFile(t, strayBranchFile, "conteudo-b")
	writeFile(t, strayFile, "eu nao deveria ser apagado")

	infoA, err := os.Stat(emptyBranchFile)
	if err != nil {
		t.Fatalf("stat a: %v", err)
	}
	infoB, err := os.Stat(strayBranchFile)
	if err != nil {
		t.Fatalf("stat b: %v", err)
	}

	m := Manifest{
		ID:        "teste-dirs-vazios",
		OutputDir: outputDir,
		Action:    ActionCopy,
		Entries: []Entry{
			{Source: "/x/a.pdf", Dest: emptyBranchFile, Size: infoA.Size()},
			{Source: "/x/b.pdf", Dest: strayBranchFile, Size: infoB.Size()},
		},
	}

	result, err := Undo(m, false, false)
	if err != nil {
		t.Fatalf("Undo() erro inesperado: %v", err)
	}
	if len(result.Restored) != 2 {
		t.Fatalf("esperava 2 restaurados, obteve %d (skipped=%+v)", len(result.Restored), result.Skipped)
	}

	// O ramo inteiro de "fornecedorA" deveria ter sido removido — ficou
	// vazio depois de apagar o único arquivo.
	mustNotExist(t, filepath.Join(outputDir, "fornecedorA"))

	// O ramo de "fornecedorB" preserva a pasta "filial1" (tem o arquivo
	// estranho dentro), mas o arquivo b.pdf, esse sim, foi apagado.
	mustNotExist(t, strayBranchFile)
	mustExist(t, strayFile)
	mustExist(t, filepath.Join(outputDir, "fornecedorB", "filial1"))
}

// TestUndoDryRunDoesNotTouchAnything confirma que dryRun apenas calcula o
// plano, sem apagar nem mover nada.
func TestUndoDryRunDoesNotTouchAnything(t *testing.T) {
	outputDir := t.TempDir()
	dest := filepath.Join(outputDir, "a.pdf")
	writeFile(t, dest, "conteudo-a")

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	m := Manifest{
		ID:        "teste-dry-run",
		OutputDir: outputDir,
		Action:    ActionCopy,
		Entries: []Entry{
			{Source: "/x/a.pdf", Dest: dest, Size: info.Size()},
		},
	}

	result, err := Undo(m, true, false)
	if err != nil {
		t.Fatalf("Undo() erro inesperado: %v", err)
	}
	if len(result.Restored) != 1 {
		t.Fatalf("esperava 1 no plano de restauração, obteve %d", len(result.Restored))
	}
	if !result.DryRun {
		t.Fatal("DryRun deveria ser true no resultado")
	}

	mustExist(t, dest) // dry-run não apaga nada de verdade
}

// TestUndoAlreadyUndoneRefusedWithoutForceAcceptedWithForce cobre a regra:
// um manifesto já desfeito não pode ser desfeito de novo sem --force.
func TestUndoAlreadyUndoneRefusedWithoutForceAcceptedWithForce(t *testing.T) {
	outputDir := t.TempDir()
	dest := filepath.Join(outputDir, "a.pdf")
	writeFile(t, dest, "conteudo-a")

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	when := time.Now()
	m := Manifest{
		ID:        "teste-ja-desfeito",
		OutputDir: outputDir,
		Action:    ActionCopy,
		UndoneAt:  &when,
		Entries: []Entry{
			{Source: "/x/a.pdf", Dest: dest, Size: info.Size()},
		},
	}

	if _, err := Undo(m, false, false); !errors.Is(err, ErrAlreadyUndone) {
		t.Fatalf("Undo() sem force em manifesto já desfeito: err = %v, esperava ErrAlreadyUndone", err)
	}
	mustExist(t, dest) // nada foi tocado na recusa

	result, err := Undo(m, false, true)
	if err != nil {
		t.Fatalf("Undo() com force erro inesperado: %v", err)
	}
	if len(result.Restored) != 1 {
		t.Fatalf("esperava 1 restaurado com force=true, obteve %d", len(result.Restored))
	}
}

// --- Outcome/Summary/BuildUndoReport ---------------------------------------
//
// Estes testes travam a correção de apresentação pedida após revisão do
// PR: a palavra "simulação" só pode aparecer quando o usuário pediu
// --dry-run explicitamente (BuildUndoReport com previewRequested=true);
// "nada a fazer" é reservado para quando não havia mesmo nada a desfazer
// (todas as entradas já ausentes do destino), nunca para quando arquivos
// foram deliberadamente preservados por segurança; e o resumo nunca deve
// ser composto duas vezes (BuildUndoReport monta exatamente um relatório
// por chamada).

func entrySkippedFor(reason SkipReason) SkippedEntry {
	return SkippedEntry{Entry: Entry{Dest: "/destino/x.pdf"}, Reason: reason}
}

// TestOutcomeNeverMentionsSimulacao confere que Outcome() — usado sempre
// que o resultado reportado NÃO é uma prévia pedida pelo usuário — jamais
// inclui a palavra "simulação", independente de r.DryRun (que reflete só
// como Undo foi internamente chamado, não se o usuário pediu uma prévia).
func TestOutcomeNeverMentionsSimulacao(t *testing.T) {
	cases := []UndoResult{
		{DryRun: true, Restored: []Entry{{Dest: "/a.pdf"}}, Skipped: nil},
		{DryRun: false, Restored: []Entry{{Dest: "/a.pdf"}}, Skipped: nil},
		{DryRun: true, Restored: nil, Skipped: nil},
		{DryRun: true, Restored: nil, Skipped: []SkippedEntry{entrySkippedFor(SkipMissing)}},
		{DryRun: true, Restored: nil, Skipped: []SkippedEntry{entrySkippedFor(SkipSizeChanged)}},
		{DryRun: true, Restored: nil, Skipped: []SkippedEntry{entrySkippedFor(SkipSourceExists)}},
	}

	for i, r := range cases {
		got := r.Outcome()
		if strings.Contains(strings.ToLower(got), "simula") {
			t.Errorf("caso %d: Outcome() = %q não deveria mencionar simulação", i, got)
		}
	}
}

// TestSummaryMentionsSimulacaoOnlyWhenDryRun confere o outro lado: Summary
// SÓ inclui o rótulo quando r.DryRun é true, e nunca quando é false — é o
// método reservado para reportar uma prévia --dry-run pedida de propósito.
func TestSummaryMentionsSimulacaoOnlyWhenDryRun(t *testing.T) {
	dryRun := UndoResult{DryRun: true, Restored: []Entry{{Dest: "/a.pdf"}}}
	if !strings.Contains(dryRun.Summary(), "simulação") {
		t.Errorf("Summary() com DryRun=true = %q, esperava mencionar \"simulação\"", dryRun.Summary())
	}

	real := UndoResult{DryRun: false, Restored: []Entry{{Dest: "/a.pdf"}}}
	if strings.Contains(real.Summary(), "simulação") {
		t.Errorf("Summary() com DryRun=false = %q, não deveria mencionar \"simulação\"", real.Summary())
	}
}

// TestOutcomeAllSkippedMissingSaysNadaAFazer é o caso em que "nada a
// fazer" É a descrição correta: todas as entradas já estavam ausentes do
// destino, não havia mesmo o que desfazer.
func TestOutcomeAllSkippedMissingSaysNadaAFazer(t *testing.T) {
	r := UndoResult{
		Restored: nil,
		Skipped: []SkippedEntry{
			entrySkippedFor(SkipMissing),
			entrySkippedFor(SkipMissing),
		},
	}

	got := r.Outcome()
	if !strings.Contains(got, "nada a fazer") {
		t.Errorf("Outcome() = %q, esperava conter \"nada a fazer\" (todas as entradas já ausentes)", got)
	}
}

// TestOutcomeSkippedForSafetyDoesNotSayNadaAFazer é o defeito relatado na
// revisão: quando arquivos foram preservados por uma decisão de
// segurança (tamanho mudou, ou origem ocupada), a mensagem final NÃO pode
// ser "nada a fazer" — havia o que fazer, e a ferramenta escolheu não
// fazer.
func TestOutcomeSkippedForSafetyDoesNotSayNadaAFazer(t *testing.T) {
	cases := []struct {
		name    string
		skipped []SkippedEntry
	}{
		{"apenas tamanho mudou", []SkippedEntry{entrySkippedFor(SkipSizeChanged)}},
		{"apenas origem ocupada", []SkippedEntry{entrySkippedFor(SkipSourceExists)}},
		{"misto: ausente + tamanho mudou", []SkippedEntry{entrySkippedFor(SkipMissing), entrySkippedFor(SkipSizeChanged)}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := UndoResult{Restored: nil, Skipped: tc.skipped}
			got := r.Outcome()
			if strings.Contains(got, "nada a fazer") {
				t.Errorf("Outcome() = %q não deveria dizer \"nada a fazer\" quando algo foi preservado por segurança", got)
			}
			if !strings.Contains(got, "preservad") {
				t.Errorf("Outcome() = %q deveria deixar claro que os arquivos foram preservados por decisão, não ausentes", got)
			}
		})
	}
}

// TestBuildUndoReportPreviewRequestedMentionsSimulacao confere que o
// rótulo "[simulação]" só aparece quando previewRequested é true — o
// caminho usado exclusivamente por uma chamada explícita com --dry-run.
func TestBuildUndoReportPreviewRequestedMentionsSimulacao(t *testing.T) {
	plan := UndoResult{DryRun: true, Restored: []Entry{{Dest: "/a.pdf"}}}

	report := BuildUndoReport(plan, true, nil)
	if !strings.Contains(report.Summary, "simulação") {
		t.Errorf("BuildUndoReport(..., previewRequested=true, ...).Summary = %q, esperava mencionar \"simulação\"", report.Summary)
	}
}

// TestBuildUndoReportRealExecutionNeverMentionsSimulacao é o teste central
// pedido na revisão: nem o caminho "nada a restaurar, encerrado antes da
// confirmação" (final=nil) nem o caminho "executado de verdade" (final
// preenchido) podem, em NENHUMA circunstância, produzir uma linha
// contendo "simulação" quando previewRequested é false — que é sempre o
// caso de uma execução real (undo sem --dry-run).
func TestBuildUndoReportRealExecutionNeverMentionsSimulacao(t *testing.T) {
	plan := UndoResult{
		DryRun:   true, // o plano SEMPRE roda em modo simulado internamente
		Restored: nil,
		Skipped:  []SkippedEntry{entrySkippedFor(SkipSizeChanged), entrySkippedFor(SkipSourceExists)},
	}

	// Caminho 1: nada a restaurar, comando encerra antes de confirmar.
	reportNothingToDo := BuildUndoReport(plan, false, nil)
	for _, line := range reportNothingToDo.Lines() {
		if strings.Contains(strings.ToLower(line), "simula") {
			t.Errorf("linha %q não deveria mencionar simulação (final=nil, previewRequested=false)", line)
		}
	}

	// Caminho 2: execução real, algo foi de fato restaurado.
	final := UndoResult{
		DryRun:   false,
		Restored: []Entry{{Dest: "/a.pdf"}},
		Skipped:  []SkippedEntry{entrySkippedFor(SkipSizeChanged)},
	}
	reportExecuted := BuildUndoReport(plan, false, &final)
	for _, line := range reportExecuted.Lines() {
		if strings.Contains(strings.ToLower(line), "simula") {
			t.Errorf("linha %q não deveria mencionar simulação (execução real)", line)
		}
	}
}

// TestBuildUndoReportSkippedForSafetyDoesNotEndWithNadaAFazer é o defeito
// 2 relatado na revisão, agora travado no nível da função pura: quando o
// comando roda de verdade (previewRequested=false) e não restaura nada
// porque os arquivos foram preservados por segurança (não porque já
// estavam ausentes), o relatório final não pode dizer "Nada a fazer.".
func TestBuildUndoReportSkippedForSafetyDoesNotEndWithNadaAFazer(t *testing.T) {
	plan := UndoResult{
		DryRun:   true,
		Restored: nil,
		Skipped:  []SkippedEntry{entrySkippedFor(SkipSizeChanged), entrySkippedFor(SkipSourceExists)},
	}

	report := BuildUndoReport(plan, false, nil)
	if strings.Contains(report.Summary, "Nada a fazer") {
		t.Fatalf("Summary = %q não deveria dizer \"Nada a fazer\" quando os arquivos foram preservados por segurança", report.Summary)
	}
}

// TestBuildUndoReportSummaryNeverDuplicated confere a garantia central:
// para qualquer combinação de argumentos, BuildUndoReport monta EXATAMENTE
// um resumo (Report.Summary é uma única string, nunca uma lista) — a
// própria assinatura do tipo UndoReport torna impossível montar um
// relatório com dois resumos. Este teste documenta essa garantia e prova
// que Lines() nunca repete o texto do resumo entre as linhas de Skipped.
func TestBuildUndoReportSummaryNeverDuplicated(t *testing.T) {
	plan := UndoResult{
		DryRun:   true,
		Restored: nil,
		Skipped:  []SkippedEntry{entrySkippedFor(SkipSizeChanged)},
	}
	final := UndoResult{
		DryRun:   false,
		Restored: []Entry{{Dest: "/a.pdf"}, {Dest: "/b.pdf"}},
		Skipped:  nil,
	}

	for _, tc := range []struct {
		name             string
		previewRequested bool
		final            *UndoResult
	}{
		{"prévia pedida", true, nil},
		{"nada a restaurar", false, nil},
		{"execução real", false, &final},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := BuildUndoReport(plan, tc.previewRequested, tc.final)
			lines := report.Lines()

			if len(lines) == 0 {
				t.Fatal("Lines() não deveria ser vazio")
			}
			// A última linha é sempre o resumo; nenhuma linha anterior
			// (as de Skipped) pode repetir esse mesmo texto.
			summary := lines[len(lines)-1]
			if summary != report.Summary {
				t.Fatalf("última linha de Lines() = %q, esperava que fosse Report.Summary = %q", summary, report.Summary)
			}
			for _, line := range lines[:len(lines)-1] {
				if line == summary {
					t.Fatalf("resumo %q apareceu duplicado em Lines(): %v", summary, lines)
				}
			}
		})
	}
}
