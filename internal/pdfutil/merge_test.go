package pdfutil

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// touchFile cria um arquivo vazio em path, criando diretórios pais conforme
// necessário.
func touchFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("criar diretório para %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatalf("criar arquivo %q: %v", path, err)
	}
}

// buildTree cria a árvore de diretórios usada pelos testes de ResolveInputs:
//
//	root/
//	  a.pdf
//	  b.PDF
//	  notes.txt
//	  sub1/
//	    c.pdf
//	    sub2/
//	      d.pdf
//	      sub3/
//	        e.pdf
func buildTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	touchFile(t, filepath.Join(root, "a.pdf"))
	touchFile(t, filepath.Join(root, "b.PDF"))
	touchFile(t, filepath.Join(root, "notes.txt"))
	touchFile(t, filepath.Join(root, "sub1", "c.pdf"))
	touchFile(t, filepath.Join(root, "sub1", "sub2", "d.pdf"))
	touchFile(t, filepath.Join(root, "sub1", "sub2", "sub3", "e.pdf"))
	return root
}

func TestResolveInputsMaxDepth0(t *testing.T) {
	root := buildTree(t)

	got, err := ResolveInputs([]string{root}, 0, "name")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("esperava 2 arquivos (a.pdf, b.PDF) no nível 0, obteve %d: %v", len(got), got)
	}
}

func TestResolveInputsMaxDepth1(t *testing.T) {
	root := buildTree(t)

	got, err := ResolveInputs([]string{root}, 1, "name")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("esperava 3 arquivos (a.pdf, b.PDF, sub1/c.pdf) na profundidade 1, obteve %d: %v", len(got), got)
	}
}

func TestResolveInputsMaxDepth2(t *testing.T) {
	root := buildTree(t)

	got, err := ResolveInputs([]string{root}, 2, "name")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("esperava 4 arquivos até a profundidade 2, obteve %d: %v", len(got), got)
	}
}

func TestResolveInputsUnlimitedDepth(t *testing.T) {
	root := buildTree(t)

	got, err := ResolveInputs([]string{root}, -1, "name")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("esperava 5 arquivos com profundidade ilimitada, obteve %d: %v", len(got), got)
	}
}

func TestResolveInputsDeduplication(t *testing.T) {
	root := buildTree(t)
	aPath := filepath.Join(root, "a.pdf")

	// a.pdf informado diretamente e também via a pasta raiz (nível 0).
	got, err := ResolveInputs([]string{aPath, root}, 0, "name")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("esperava 2 arquivos únicos (a.pdf deduplicado, b.PDF), obteve %d: %v", len(got), got)
	}

	count := 0
	for _, f := range got {
		abs, _ := filepath.Abs(f)
		if filepath.Clean(abs) == filepath.Clean(aPath) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("a.pdf deveria aparecer exatamente 1 vez, apareceu %d", count)
	}
}

func TestResolveInputsSortByName(t *testing.T) {
	root := t.TempDir()
	touchFile(t, filepath.Join(root, "z.pdf"))
	touchFile(t, filepath.Join(root, "a.pdf"))
	touchFile(t, filepath.Join(root, "m.pdf"))

	got, err := ResolveInputs([]string{root}, 0, "name")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("esperava 3 arquivos, obteve %d", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Fatalf("resultado não está ordenado por nome: %v", got)
		}
	}
}

func TestResolveInputsSortByMtime(t *testing.T) {
	root := t.TempDir()
	older := filepath.Join(root, "z-older.pdf")
	newer := filepath.Join(root, "a-newer.pdf")

	touchFile(t, older)

	// Ajusta os tempos de modificação explicitamente para garantir a ordem
	// esperada, independente da velocidade de execução do teste.
	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	future := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(older, past, past); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	touchFile(t, newer)
	if err := os.Chtimes(newer, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	got, err := ResolveInputs([]string{root}, 0, "mtime")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("esperava 2 arquivos, obteve %d", len(got))
	}
	if filepath.Base(got[0]) != "z-older.pdf" || filepath.Base(got[1]) != "a-newer.pdf" {
		t.Fatalf("ordenação por mtime incorreta: %v", got)
	}
}

func TestResolveInputsMissingEntry(t *testing.T) {
	_, err := ResolveInputs([]string{"/caminho/que/nao/existe/em/lugar/nenhum"}, 0, "name")
	if err == nil {
		t.Fatal("esperava erro para entrada inexistente")
	}
}

func TestResolveInputsMixFileAndDir(t *testing.T) {
	root := buildTree(t)
	otherDir := t.TempDir()
	otherFile := filepath.Join(otherDir, "solo.pdf")
	touchFile(t, otherFile)

	got, err := ResolveInputs([]string{otherFile, root}, 0, "name")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	// root nível 0: a.pdf, b.PDF + otherFile solo.pdf = 3
	if len(got) != 3 {
		t.Fatalf("esperava 3 arquivos misturando arquivo solto e diretório, obteve %d: %v", len(got), got)
	}
}

func TestMergeNoInputsFound(t *testing.T) {
	root := t.TempDir()
	touchFile(t, filepath.Join(root, "notes.txt"))

	_, err := Merge(context.Background(), MergeOptions{
		Inputs: []string{root},
		Output: filepath.Join(root, "out.pdf"),
	})
	if err == nil {
		t.Fatal("esperava erro quando nenhum PDF é encontrado")
	}
}

func TestMergeOutputExistsWithoutOverwrite(t *testing.T) {
	root := buildTree(t)
	out := filepath.Join(root, "existing.pdf")
	touchFile(t, out)

	_, err := Merge(context.Background(), MergeOptions{
		Inputs:    []string{root},
		Output:    out,
		Overwrite: false,
	})
	if err == nil {
		t.Fatal("esperava erro pedindo --overwrite quando o output já existe")
	}
}
