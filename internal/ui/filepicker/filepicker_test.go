package filepicker

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestListDir_FilterByExtension(t *testing.T) {
	tmpDir := t.TempDir()

	// Cria estrutura de teste
	subdir1 := filepath.Join(tmpDir, "dir1")
	subdir2 := filepath.Join(tmpDir, "dir2")
	os.Mkdir(subdir1, 0755)
	os.Mkdir(subdir2, 0755)

	// Arquivo PDF 1
	os.WriteFile(filepath.Join(tmpDir, "document1.pdf"), []byte("pdf content"), 0644)
	// Arquivo PDF 2
	os.WriteFile(filepath.Join(tmpDir, "document2.pdf"), []byte("pdf content"), 0644)
	// Arquivo TXT
	os.WriteFile(filepath.Join(tmpDir, "notes.txt"), []byte("txt content"), 0644)
	// Arquivo oculto PDF
	os.WriteFile(filepath.Join(tmpDir, ".hidden.pdf"), []byte("hidden pdf"), 0644)
	// Diretório oculto
	os.Mkdir(filepath.Join(tmpDir, ".git"), 0755)

	entries, err := ListDir(tmpDir, []string{".pdf"})
	if err != nil {
		t.Fatalf("ListDir devolveu erro: %v", err)
	}

	// Deve devolver 2 diretórios + 2 PDFs = 4 entradas
	if len(entries) != 4 {
		t.Errorf("esperava 4 entradas, obteve %d", len(entries))
	}

	// Verifica que diretórios vêm primeiro
	if !entries[0].IsDir || entries[0].Name != "dir1" {
		t.Errorf("primeira entrada deve ser dir1 (diretório), obteve %s (IsDir=%v)", entries[0].Name, entries[0].IsDir)
	}

	if !entries[1].IsDir || entries[1].Name != "dir2" {
		t.Errorf("segunda entrada deve ser dir2 (diretório), obteve %s (IsDir=%v)", entries[1].Name, entries[1].IsDir)
	}

	// Verifica PDFs
	if entries[2].IsDir || entries[2].Name != "document1.pdf" {
		t.Errorf("terceira entrada deve ser document1.pdf, obteve %s", entries[2].Name)
	}

	if entries[3].IsDir || entries[3].Name != "document2.pdf" {
		t.Errorf("quarta entrada deve ser document2.pdf, obteve %s", entries[3].Name)
	}

	// Arquivo oculto não deve aparecer
	for _, e := range entries {
		if e.Name == ".hidden.pdf" {
			t.Errorf("arquivo oculto .hidden.pdf não deveria aparecer")
		}
	}
}

func TestListDir_NoFilter(t *testing.T) {
	tmpDir := t.TempDir()

	os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "file1.pdf"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(tmpDir, ".hidden"), []byte("hidden"), 0644)

	// exts nil = todos os arquivos
	entries, err := ListDir(tmpDir, nil)
	if err != nil {
		t.Fatalf("ListDir devolveu erro: %v", err)
	}

	// Deve devolver 1 diretório + 2 arquivos = 3 entradas
	if len(entries) != 3 {
		t.Errorf("esperava 3 entradas, obteve %d", len(entries))
	}

	// Verifica que .hidden não aparece
	for _, e := range entries {
		if e.Name == ".hidden" {
			t.Errorf("arquivo oculto .hidden não deveria aparecer")
		}
	}

	// Verifica que pdf e txt aparecem
	hasPDF := false
	hasTXT := false
	for _, e := range entries {
		if e.Name == "file1.pdf" {
			hasPDF = true
		}
		if e.Name == "file2.txt" {
			hasTXT = true
		}
	}

	if !hasPDF {
		t.Errorf("file1.pdf deveria aparecer")
	}
	if !hasTXT {
		t.Errorf("file2.txt deveria aparecer")
	}
}

func TestListDir_DirectoriesFirst(t *testing.T) {
	tmpDir := t.TempDir()

	os.Mkdir(filepath.Join(tmpDir, "zebra"), 0755)
	os.Mkdir(filepath.Join(tmpDir, "apple"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "zzz.txt"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "aaa.txt"), []byte("content"), 0644)

	entries, err := ListDir(tmpDir, nil)
	if err != nil {
		t.Fatalf("ListDir devolveu erro: %v", err)
	}

	// Verifica ordem: apple, zebra (diretórios), aaa.txt, zzz.txt (arquivos)
	expected := []string{"apple", "zebra", "aaa.txt", "zzz.txt"}
	for i, name := range expected {
		if entries[i].Name != name {
			t.Errorf("entrada %d: esperava %s, obteve %s", i, name, entries[i].Name)
		}
	}

	// Verifica que apple e zebra são diretórios
	if !entries[0].IsDir || !entries[1].IsDir {
		t.Errorf("primeiras duas entradas devem ser diretórios")
	}

	// Verifica que aaa.txt e zzz.txt não são diretórios
	if entries[2].IsDir || entries[3].IsDir {
		t.Errorf("últimas duas entradas devem ser arquivos")
	}
}

func TestListDir_CaseInsensitiveExtension(t *testing.T) {
	tmpDir := t.TempDir()

	os.WriteFile(filepath.Join(tmpDir, "ARQUIVO.PDF"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "documento.pdf"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "nota.txt"), []byte("content"), 0644)

	entries, err := ListDir(tmpDir, []string{".pdf"})
	if err != nil {
		t.Fatalf("ListDir devolveu erro: %v", err)
	}

	// Deve retornar os dois PDFs independente de maiúscula/minúscula
	if len(entries) != 2 {
		t.Errorf("esperava 2 entradas .pdf, obteve %d", len(entries))
	}

	hasUpper := false
	hasLower := false
	for _, e := range entries {
		if e.Name == "ARQUIVO.PDF" {
			hasUpper = true
		}
		if e.Name == "documento.pdf" {
			hasLower = true
		}
	}

	if !hasUpper {
		t.Errorf("ARQUIVO.PDF não foi encontrado")
	}
	if !hasLower {
		t.Errorf("documento.pdf não foi encontrado")
	}
}

func TestListDir_AbsolutePaths(t *testing.T) {
	tmpDir := t.TempDir()

	os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("content"), 0644)

	entries, err := ListDir(tmpDir, nil)
	if err != nil {
		t.Fatalf("ListDir devolveu erro: %v", err)
	}

	for _, e := range entries {
		if !filepath.IsAbs(e.Path) {
			t.Errorf("caminho %s não é absoluto", e.Path)
		}
	}
}

func TestListDir_NonexistentDirectory(t *testing.T) {
	nonexistent := "/tmp/this_directory_does_not_exist_xyz123456789"

	_, err := ListDir(nonexistent, nil)
	if err == nil {
		t.Errorf("esperava erro ao ler diretório inexistente, mas obteve nil")
	}
}

// TestListDir_CountFilesVsDirs cobre a contagem de arquivos versus
// diretórios que a correção de "pasta de origem sem PDFs" do organize-pdf
// depende: ListDir(dir, []string{".pdf"}) deve devolver os subdiretórios
// (todos, sem filtro de extensão) mais só os arquivos .pdf, e contar
// entradas com IsDir == false deve bater com a quantidade real de PDFs.
func TestListDir_CountFilesVsDirs(t *testing.T) {
	tmpDir := t.TempDir()

	os.Mkdir(filepath.Join(tmpDir, "subpasta1"), 0755)
	os.Mkdir(filepath.Join(tmpDir, "subpasta2"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "a.pdf"), []byte("pdf"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "b.pdf"), []byte("pdf"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "c.pdf"), []byte("pdf"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "notas.txt"), []byte("txt"), 0644)

	entries, err := ListDir(tmpDir, []string{".pdf"})
	if err != nil {
		t.Fatalf("ListDir devolveu erro: %v", err)
	}

	var dirCount, fileCount int
	for _, e := range entries {
		if e.IsDir {
			dirCount++
		} else {
			fileCount++
		}
	}

	if dirCount != 2 {
		t.Errorf("esperava 2 diretórios, obteve %d", dirCount)
	}
	if fileCount != 3 {
		t.Errorf("esperava 3 arquivos .pdf, obteve %d", fileCount)
	}
}

// TestListDir_EmptyDirectoryReturnsZeroFiles cobre o caso de pasta de
// origem vazia: nenhuma entrada, nenhum erro.
func TestListDir_EmptyDirectoryReturnsZeroFiles(t *testing.T) {
	tmpDir := t.TempDir()

	entries, err := ListDir(tmpDir, []string{".pdf"})
	if err != nil {
		t.Fatalf("ListDir devolveu erro: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("esperava 0 entradas em diretório vazio, obteve %d", len(entries))
	}
}

// TestPickFileSignatureUnchanged trava, em tempo de compilação, que PickFile
// mantém a assinatura original — outras ferramentas (mergepdf, splitpdf)
// dependem dela e não podem quebrar com a introdução de PickFileWithPrompt.
func TestPickFileSignatureUnchanged(t *testing.T) {
	var _ func(start string, exts []string) (string, error) = PickFile
}

// TestPickDirSignatureUnchanged trava, em tempo de compilação, que PickDir
// mantém a assinatura original — outras ferramentas dependem dela e não
// podem quebrar com a introdução de PickDirWithPrompt.
func TestPickDirSignatureUnchanged(t *testing.T) {
	var _ func(start string) (string, error) = PickDir
}

// TestSetLastDirAndLastDir cobre o registro básico: sem nenhuma seleção
// concluída, LastDir() é ""; depois de setLastDir, LastDir() reflete o
// valor registrado.
func TestSetLastDirAndLastDir(t *testing.T) {
	ResetLastDir()
	defer ResetLastDir()

	if got := LastDir(); got != "" {
		t.Fatalf("LastDir() antes de qualquer seleção = %q, want \"\"", got)
	}

	setLastDir("/tmp/algum/lugar")

	if got := LastDir(); got != "/tmp/algum/lugar" {
		t.Errorf("LastDir() = %q, want %q", got, "/tmp/algum/lugar")
	}
}

// TestResetLastDirClearsState garante que ResetLastDir realmente limpa a
// memória — os testes dependem disso para não se contaminarem uns aos
// outros.
func TestResetLastDirClearsState(t *testing.T) {
	setLastDir("/tmp/outro/lugar")
	ResetLastDir()

	if got := LastDir(); got != "" {
		t.Errorf("LastDir() após ResetLastDir() = %q, want \"\"", got)
	}
}

// TestResolveStartExplicitStartWinsOverMemory é a regra mais importante da
// memória de último diretório: um start explícito e não-vazio nunca pode
// ser silenciosamente trocado pela memória.
func TestResolveStartExplicitStartWinsOverMemory(t *testing.T) {
	ResetLastDir()
	defer ResetLastDir()

	tmpDir := t.TempDir()
	otherDir := t.TempDir()
	setLastDir(otherDir)

	got, err := resolveStart(tmpDir)
	if err != nil {
		t.Fatalf("resolveStart() erro inesperado: %v", err)
	}
	want, _ := filepath.Abs(tmpDir)
	if got != want {
		t.Errorf("resolveStart(%q) = %q, want %q (start explícito deveria vencer a memória)", tmpDir, got, want)
	}
}

// TestResolveStartEmptyUsesMemoryWhenPresent cobre o caso que motivou a
// memória: start vazio deve continuar da última pasta visitada.
func TestResolveStartEmptyUsesMemoryWhenPresent(t *testing.T) {
	ResetLastDir()
	defer ResetLastDir()

	tmpDir := t.TempDir()
	setLastDir(tmpDir)

	got, err := resolveStart("")
	if err != nil {
		t.Fatalf("resolveStart() erro inesperado: %v", err)
	}
	want, _ := filepath.Abs(tmpDir)
	if got != want {
		t.Errorf("resolveStart(\"\") = %q, want %q (deveria usar a memória)", got, want)
	}
}

// TestResolveStartEmptyWithoutMemoryFallsBackToWorkingDir cobre o
// comportamento histórico: sem start e sem memória ainda registrada, cai no
// diretório de trabalho atual.
func TestResolveStartEmptyWithoutMemoryFallsBackToWorkingDir(t *testing.T) {
	ResetLastDir()
	defer ResetLastDir()

	got, err := resolveStart("")
	if err != nil {
		t.Fatalf("resolveStart() erro inesperado: %v", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("erro ao obter diretório de trabalho: %v", err)
	}
	if got != wd {
		t.Errorf("resolveStart(\"\") sem memória = %q, want %q (diretório de trabalho atual)", got, wd)
	}
}

// TestSetLastDirConcurrentNoRace cobre chamadas concorrentes ao registro de
// último diretório: a suíte roda com -race, então qualquer acesso
// desprotegido a lastDir seria detectado aqui.
func TestSetLastDirConcurrentNoRace(t *testing.T) {
	ResetLastDir()
	defer ResetLastDir()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			setLastDir("/tmp/concorrente")
		}()
		go func() {
			defer wg.Done()
			_ = LastDir()
		}()
	}
	wg.Wait()
}
