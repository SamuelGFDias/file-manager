package history

import (
	"errors"
	"os"
	"path/filepath"
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
