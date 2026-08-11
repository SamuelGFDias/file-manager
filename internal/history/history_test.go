package history

import (
	"testing"
	"time"
)

// withTempConfigDir substitui userConfigDir por um diretório temporário
// durante o teste, restaurando o valor original ao final — mesmo padrão
// usado em internal/config (internal/config/profile_test.go).
func withTempConfigDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	original := userConfigDir
	userConfigDir = func() (string, error) {
		return dir, nil
	}
	t.Cleanup(func() {
		userConfigDir = original
	})

	return dir
}

func sampleManifest() Manifest {
	return Manifest{
		Tool:      "organize-pdf",
		InputDir:  "/tmp/entrada",
		OutputDir: "/tmp/saida",
		Action:    ActionCopy,
		Entries: []Entry{
			{Source: "/tmp/entrada/a.pdf", Dest: "/tmp/saida/a.pdf", Size: 100},
		},
	}
}

// TestSaveGeneratesIDAndLoadRoundTrips é o round-trip central: Save() sem ID
// explícito gera um, e Load() com esse ID devolve exatamente o que foi
// gravado.
func TestSaveGeneratesIDAndLoadRoundTrips(t *testing.T) {
	withTempConfigDir(t)

	m := sampleManifest()
	path, err := Save(m)
	if err != nil {
		t.Fatalf("Save() erro inesperado: %v", err)
	}
	if path == "" {
		t.Fatal("Save() devolveu caminho vazio")
	}

	manifests, err := List()
	if err != nil {
		t.Fatalf("List() erro inesperado: %v", err)
	}
	if len(manifests) != 1 {
		t.Fatalf("List() devolveu %d manifestos, esperava 1", len(manifests))
	}

	saved := manifests[0]
	if saved.ID == "" {
		t.Fatal("Save() deveria ter gerado um ID não vazio")
	}
	if saved.Tool != m.Tool || saved.InputDir != m.InputDir || saved.OutputDir != m.OutputDir {
		t.Fatalf("manifesto carregado difere do gravado: %+v vs %+v", saved, m)
	}
	if len(saved.Entries) != 1 || saved.Entries[0] != m.Entries[0] {
		t.Fatalf("Entries não bateram: got %+v, want %+v", saved.Entries, m.Entries)
	}

	loaded, err := Load(saved.ID)
	if err != nil {
		t.Fatalf("Load(%q) erro inesperado: %v", saved.ID, err)
	}
	if loaded.ID != saved.ID {
		t.Fatalf("Load().ID = %q, esperava %q", loaded.ID, saved.ID)
	}
}

func TestSaveWithExplicitIDCollisionGetsSuffix(t *testing.T) {
	withTempConfigDir(t)

	when := time.Date(2026, 8, 11, 16, 45, 30, 0, time.Local)

	first := sampleManifest()
	first.CreatedAt = when
	if _, err := Save(first); err != nil {
		t.Fatalf("Save() (primeiro) erro inesperado: %v", err)
	}

	second := sampleManifest()
	second.CreatedAt = when
	if _, err := Save(second); err != nil {
		t.Fatalf("Save() (segundo) erro inesperado: %v", err)
	}

	manifests, err := List()
	if err != nil {
		t.Fatalf("List() erro inesperado: %v", err)
	}
	if len(manifests) != 2 {
		t.Fatalf("List() devolveu %d manifestos, esperava 2 (mesmo CreatedAt, IDs diferentes)", len(manifests))
	}
	if manifests[0].ID == manifests[1].ID {
		t.Fatalf("os dois manifestos gravados com o mesmo CreatedAt não deveriam ter o mesmo ID: %q", manifests[0].ID)
	}
}

func TestListReturnsEmptySliceWhenDirMissing(t *testing.T) {
	withTempConfigDir(t)

	manifests, err := List()
	if err != nil {
		t.Fatalf("List() erro inesperado quando o diretório de histórico não existe: %v", err)
	}
	if len(manifests) != 0 {
		t.Fatalf("List() devolveu %d manifestos, esperava 0", len(manifests))
	}
}

func TestListNewestFirst(t *testing.T) {
	withTempConfigDir(t)

	older := sampleManifest()
	older.CreatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	if _, err := Save(older); err != nil {
		t.Fatalf("Save() (older) erro inesperado: %v", err)
	}

	newer := sampleManifest()
	newer.CreatedAt = time.Date(2026, 6, 1, 0, 0, 0, 0, time.Local)
	if _, err := Save(newer); err != nil {
		t.Fatalf("Save() (newer) erro inesperado: %v", err)
	}

	manifests, err := List()
	if err != nil {
		t.Fatalf("List() erro inesperado: %v", err)
	}
	if len(manifests) != 2 {
		t.Fatalf("List() devolveu %d manifestos, esperava 2", len(manifests))
	}
	if !manifests[0].CreatedAt.After(manifests[1].CreatedAt) {
		t.Fatalf(
			"List() não devolveu mais recentes primeiro: manifests[0].CreatedAt=%v, manifests[1].CreatedAt=%v",
			manifests[0].CreatedAt, manifests[1].CreatedAt,
		)
	}
}

func TestMarkUndoneRecordsTimestamp(t *testing.T) {
	withTempConfigDir(t)

	m := sampleManifest()
	if _, err := Save(m); err != nil {
		t.Fatalf("Save() erro inesperado: %v", err)
	}

	manifests, err := List()
	if err != nil {
		t.Fatalf("List() erro inesperado: %v", err)
	}
	id := manifests[0].ID

	if manifests[0].UndoneAt != nil {
		t.Fatal("manifesto recém-gravado não deveria ter UndoneAt preenchido")
	}

	when := time.Date(2026, 8, 11, 18, 0, 0, 0, time.Local)
	if err := MarkUndone(id, when); err != nil {
		t.Fatalf("MarkUndone() erro inesperado: %v", err)
	}

	reloaded, err := Load(id)
	if err != nil {
		t.Fatalf("Load() erro inesperado: %v", err)
	}
	if reloaded.UndoneAt == nil {
		t.Fatal("UndoneAt deveria estar preenchido após MarkUndone")
	}
	if !reloaded.UndoneAt.Equal(when) {
		t.Fatalf("UndoneAt = %v, esperava %v", *reloaded.UndoneAt, when)
	}
	// O ID não pode mudar quando MarkUndone regrava o manifesto.
	if reloaded.ID != id {
		t.Fatalf("ID mudou depois de MarkUndone: %q -> %q", id, reloaded.ID)
	}
}

func TestLoadNonexistentIDErrors(t *testing.T) {
	withTempConfigDir(t)

	if _, err := Load("nao-existe"); err == nil {
		t.Fatal("Load() de um ID inexistente deveria devolver erro")
	}
}

func TestDirComposesUserConfigDirWithAppName(t *testing.T) {
	base := withTempConfigDir(t)

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() erro inesperado: %v", err)
	}

	want := base + "/file-manager/history"
	if dir != want {
		// filepath.Join usa o separador do SO; em Linux (ambiente de teste
		// e2e/make e2e) bate exatamente com a comparação acima.
		t.Errorf("Dir() = %q, esperava %q", dir, want)
	}
}
