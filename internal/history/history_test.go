package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// writeManifestRaw grava m diretamente no disco, IGNORANDO a poda
// automática que Save() sempre dispara em seguida (com os limiares REAIS de
// PruneUndoneAfter/PrunePendingAfter, não os valores customizados que os
// testes de Prune() passam). Necessário para construir, de forma
// determinística, um manifesto "antigo" (ex: pendente há 200 dias) sem que
// a própria chamada de gravação já o remova antes do teste conseguir
// examiná-lo — o mesmo teria acontecido, silenciosamente, com Save() puro,
// já que a máquina que roda estes testes está com o relógio em 2026,
// tornando datas fixas de anos anteriores "antigas o bastante" para a poda
// real disparar de imediato.
func writeManifestRaw(t *testing.T, m Manifest) string {
	t.Helper()

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() erro inesperado: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("criar diretório de histórico: %v", err)
	}

	if m.ID == "" {
		id, err := generateID(dir, m.CreatedAt)
		if err != nil {
			t.Fatalf("generateID: %v", err)
		}
		m.ID = id
	}

	out, err := yaml.Marshal(&m)
	if err != nil {
		t.Fatalf("codificar manifesto: %v", err)
	}
	if err := os.WriteFile(manifestPath(dir, m.ID), out, 0o644); err != nil {
		t.Fatalf("gravar manifesto: %v", err)
	}

	return m.ID
}

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
	path, _, err := Save(m)
	if err != nil {
		t.Fatalf("Save() erro inesperado: %v", err)
	}
	if path == "" {
		t.Fatal("Save() devolveu caminho vazio")
	}

	headers, warnings, err := List()
	if err != nil {
		t.Fatalf("List() erro inesperado: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("List() devolveu warnings inesperados: %v", warnings)
	}
	if len(headers) != 1 {
		t.Fatalf("List() devolveu %d cabeçalhos, esperava 1", len(headers))
	}

	saved := headers[0]
	if saved.ID == "" {
		t.Fatal("Save() deveria ter gerado um ID não vazio")
	}
	if saved.Tool != m.Tool || saved.InputDir != m.InputDir || saved.OutputDir != m.OutputDir {
		t.Fatalf("cabeçalho carregado difere do gravado: %+v vs %+v", saved, m)
	}
	if saved.EntryCount != len(m.Entries) {
		t.Fatalf("EntryCount = %d, esperava %d", saved.EntryCount, len(m.Entries))
	}

	loaded, err := Load(saved.ID)
	if err != nil {
		t.Fatalf("Load(%q) erro inesperado: %v", saved.ID, err)
	}
	if loaded.ID != saved.ID {
		t.Fatalf("Load().ID = %q, esperava %q", loaded.ID, saved.ID)
	}
	if len(loaded.Entries) != 1 || loaded.Entries[0] != m.Entries[0] {
		t.Fatalf("Load().Entries não bateu: got %+v, want %+v", loaded.Entries, m.Entries)
	}
}

func TestSaveWithExplicitIDCollisionGetsSuffix(t *testing.T) {
	withTempConfigDir(t)

	when := time.Date(2026, 8, 11, 16, 45, 30, 0, time.Local)

	first := sampleManifest()
	first.CreatedAt = when
	if _, _, err := Save(first); err != nil {
		t.Fatalf("Save() (primeiro) erro inesperado: %v", err)
	}

	second := sampleManifest()
	second.CreatedAt = when
	if _, _, err := Save(second); err != nil {
		t.Fatalf("Save() (segundo) erro inesperado: %v", err)
	}

	headers, _, err := List()
	if err != nil {
		t.Fatalf("List() erro inesperado: %v", err)
	}
	if len(headers) != 2 {
		t.Fatalf("List() devolveu %d cabeçalhos, esperava 2 (mesmo CreatedAt, IDs diferentes)", len(headers))
	}
	if headers[0].ID == headers[1].ID {
		t.Fatalf("os dois manifestos gravados com o mesmo CreatedAt não deveriam ter o mesmo ID: %q", headers[0].ID)
	}
}

func TestListReturnsEmptySliceWhenDirMissing(t *testing.T) {
	withTempConfigDir(t)

	headers, warnings, err := List()
	if err != nil {
		t.Fatalf("List() erro inesperado quando o diretório de histórico não existe: %v", err)
	}
	if len(headers) != 0 {
		t.Fatalf("List() devolveu %d cabeçalhos, esperava 0", len(headers))
	}
	if len(warnings) != 0 {
		t.Fatalf("List() devolveu warnings inesperados para diretório inexistente: %v", warnings)
	}
}

// TestListUnreadableDirReturnsError garante que List() propaga erro quando
// o PRÓPRIO diretório de histórico não pode ser lido (ex: sem permissão) —
// diferente de um arquivo individual ilegível dentro dele, que não deve
// interromper a listagem (ver TestListSkipsCorruptedManifestAndWarns).
func TestListUnreadableDirReturnsError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("rodando como root: permissões de diretório não são respeitadas, teste não se aplica")
	}

	base := withTempConfigDir(t)
	dir := filepath.Join(base, "file-manager", "history")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("criar diretório de histórico: %v", err)
	}
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatalf("remover permissão de leitura do diretório: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o755)
	})

	if _, _, err := List(); err == nil {
		t.Fatal("List() deveria devolver erro quando o diretório de histórico não pode ser lido")
	}
}

// TestListSkipsCorruptedManifestAndWarns é o teste mais importante desta
// entrega: um manifesto ilegível (arquivo truncado/YAML inválido) não pode
// derrubar a listagem inteira, e com ela o "undo" de todas as outras
// operações. Com três arquivos no diretório e um deles corrompido, List()
// deve devolver os dois íntegros, um warning citando o arquivo problemático,
// e err == nil.
func TestListSkipsCorruptedManifestAndWarns(t *testing.T) {
	withTempConfigDir(t)

	// CreatedAt relativo a "agora" (não uma data fixa no passado): bem
	// dentro do prazo de retenção de pendentes, para que Save() não os pode
	// automaticamente antes do teste conseguir examiná-los.
	first := sampleManifest()
	first.CreatedAt = time.Now().Add(-2 * time.Hour)
	if _, _, err := Save(first); err != nil {
		t.Fatalf("Save() (first) erro inesperado: %v", err)
	}

	second := sampleManifest()
	second.CreatedAt = time.Now().Add(-1 * time.Hour)
	if _, _, err := Save(second); err != nil {
		t.Fatalf("Save() (second) erro inesperado: %v", err)
	}

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() erro inesperado: %v", err)
	}
	corruptedPath := filepath.Join(dir, "20261231-235959.yaml")
	if err := os.WriteFile(corruptedPath, []byte("isto: [nao é: yaml válido\n\tlixo binário"), 0o644); err != nil {
		t.Fatalf("criar manifesto corrompido: %v", err)
	}

	headers, warnings, err := List()
	if err != nil {
		t.Fatalf("List() não deveria devolver erro por causa de um manifesto individual corrompido: %v", err)
	}
	if len(headers) != 2 {
		t.Fatalf("List() devolveu %d cabeçalhos, esperava 2 (os dois íntegros): %+v", len(headers), headers)
	}
	if len(warnings) != 1 {
		t.Fatalf("List() devolveu %d warnings, esperava 1 (o manifesto corrompido): %v", len(warnings), warnings)
	}
	if want := "20261231-235959.yaml"; !strings.Contains(warnings[0], want) {
		t.Fatalf("warning = %q, esperava conter o nome do arquivo problemático %q", warnings[0], want)
	}
}

func TestListNewestFirst(t *testing.T) {
	withTempConfigDir(t)

	// CreatedAt relativo a "agora": ambos bem dentro do prazo de retenção
	// de pendentes, para que Save() não pode nenhum dos dois antes do
	// teste verificar a ordem.
	older := sampleManifest()
	older.CreatedAt = time.Now().Add(-2 * time.Hour)
	if _, _, err := Save(older); err != nil {
		t.Fatalf("Save() (older) erro inesperado: %v", err)
	}

	newer := sampleManifest()
	newer.CreatedAt = time.Now().Add(-1 * time.Hour)
	if _, _, err := Save(newer); err != nil {
		t.Fatalf("Save() (newer) erro inesperado: %v", err)
	}

	headers, _, err := List()
	if err != nil {
		t.Fatalf("List() erro inesperado: %v", err)
	}
	if len(headers) != 2 {
		t.Fatalf("List() devolveu %d cabeçalhos, esperava 2", len(headers))
	}
	if !headers[0].CreatedAt.After(headers[1].CreatedAt) {
		t.Fatalf(
			"List() não devolveu mais recentes primeiro: headers[0].CreatedAt=%v, headers[1].CreatedAt=%v",
			headers[0].CreatedAt, headers[1].CreatedAt,
		)
	}
}

func TestMarkUndoneRecordsTimestamp(t *testing.T) {
	withTempConfigDir(t)

	m := sampleManifest()
	if _, _, err := Save(m); err != nil {
		t.Fatalf("Save() erro inesperado: %v", err)
	}

	headers, _, err := List()
	if err != nil {
		t.Fatalf("List() erro inesperado: %v", err)
	}
	id := headers[0].ID

	if headers[0].UndoneAt != nil {
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

// saveUndone grava um manifesto e imediatamente o marca como desfeito em
// undoneAt — atalho usado pelos testes de Prune, que precisam de manifestos
// já desfeitos em datas específicas.
func saveUndone(t *testing.T, undoneAt time.Time) string {
	t.Helper()

	m := sampleManifest()
	if _, _, err := Save(m); err != nil {
		t.Fatalf("Save() erro inesperado: %v", err)
	}

	headers, _, err := List()
	if err != nil {
		t.Fatalf("List() erro inesperado: %v", err)
	}
	id := headers[0].ID

	if err := MarkUndone(id, undoneAt); err != nil {
		t.Fatalf("MarkUndone() erro inesperado: %v", err)
	}

	return id
}

// savePendingWithAge grava (via writeManifestRaw, contornando a poda
// automática de Save() — ver seu comentário) um manifesto pendente (nunca
// desfeito) com CreatedAt = now.Add(-age). Atalho para os testes de poda de
// pendentes.
func savePendingWithAge(t *testing.T, now time.Time, age time.Duration) string {
	t.Helper()

	m := sampleManifest()
	m.CreatedAt = now.Add(-age)
	return writeManifestRaw(t, m)
}

const testUndoneRetention = 30 * 24 * time.Hour
const testPendingRetention = 180 * 24 * time.Hour

func TestPruneRemovesUndoneManifestsOlderThanRetention(t *testing.T) {
	withTempConfigDir(t)

	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local)

	expiredID := saveUndone(t, now.Add(-31*24*time.Hour))

	removed, err := Prune(now, testUndoneRetention, testPendingRetention)
	if err != nil {
		t.Fatalf("Prune() erro inesperado: %v", err)
	}
	if len(removed) != 1 || removed[0] != expiredID {
		t.Fatalf("Prune() removeu %v, esperava só [%q]", removed, expiredID)
	}

	if _, err := Load(expiredID); err == nil {
		t.Fatalf("manifesto %q deveria ter sido removido do disco", expiredID)
	}
}

func TestPruneKeepsRecentlyUndoneManifests(t *testing.T) {
	withTempConfigDir(t)

	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local)

	recentID := saveUndone(t, now.Add(-1*time.Hour))

	removed, err := Prune(now, testUndoneRetention, testPendingRetention)
	if err != nil {
		t.Fatalf("Prune() erro inesperado: %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("Prune() removeu %v, esperava nada (manifesto desfeito há só 1h, retenção de 30 dias)", removed)
	}

	if _, err := Load(recentID); err != nil {
		t.Fatalf("manifesto %q não deveria ter sido removido: %v", recentID, err)
	}
}

// TestPruneRemovesExpiredPendingManifests é o item que faltava na poda
// herdada: um manifesto PENDENTE (nunca desfeito) mais antigo que
// PrunePendingAfter precisa ser removido — é o caso comum de quem organiza
// e nunca desfaz, que sem isso acumulava histórico para sempre.
func TestPruneRemovesExpiredPendingManifests(t *testing.T) {
	withTempConfigDir(t)

	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local)
	expiredID := savePendingWithAge(t, now, testPendingRetention+24*time.Hour)

	removed, err := Prune(now, testUndoneRetention, testPendingRetention)
	if err != nil {
		t.Fatalf("Prune() erro inesperado: %v", err)
	}
	if len(removed) != 1 || removed[0] != expiredID {
		t.Fatalf("Prune() removeu %v, esperava só [%q] (pendente expirado)", removed, expiredID)
	}

	if _, err := Load(expiredID); err == nil {
		t.Fatalf("manifesto pendente expirado %q deveria ter sido removido do disco", expiredID)
	}
}

// TestPruneKeepsPendingManifestsRegardlessOfAge é a garantia central de
// Prune: um manifesto NUNCA desfeito não pode ser removido automaticamente
// antes de pendingAfter, não importa o quão antigo — é a única coisa que
// permite ao usuário desfazer aquela operação mais tarde.
func TestPruneKeepsPendingManifestsRegardlessOfAge(t *testing.T) {
	withTempConfigDir(t)

	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local)

	// writeManifestRaw (não Save()) de propósito: um manifesto "de 2020"
	// já teria sido removido pela própria poda automática de Save(), que
	// sempre usa os limiares REAIS — o teste quer isolar o comportamento
	// de Prune() com um limiar customizado bem generoso, não o de Save().
	veryOld := sampleManifest()
	veryOld.CreatedAt = time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local)
	pendingID := writeManifestRaw(t, veryOld)
	// pendingAfter maior que a idade real do manifesto (6 anos): mesmo um
	// limiar generoso não deveria remover um pendente ainda "dentro do
	// prazo".
	removed, err := Prune(now, testUndoneRetention, 100*365*24*time.Hour)
	if err != nil {
		t.Fatalf("Prune() erro inesperado: %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("Prune() removeu %v, esperava nada — manifesto pendente dentro do prazo nunca é removido", removed)
	}

	if _, err := Load(pendingID); err != nil {
		t.Fatalf("manifesto pendente %q não deveria ter sido removido: %v", pendingID, err)
	}
}

// TestPruneNeverTouchesRecentPending confirma explicitamente que um
// manifesto pendente RECENTE (bem dentro do prazo) nunca é removido, nem
// por engano com um pendingAfter curto que só deveria afetar os antigos.
func TestPruneNeverTouchesRecentPending(t *testing.T) {
	withTempConfigDir(t)

	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local)
	recentID := savePendingWithAge(t, now, 1*time.Hour)

	removed, err := Prune(now, testUndoneRetention, testPendingRetention)
	if err != nil {
		t.Fatalf("Prune() erro inesperado: %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("Prune() removeu %v, esperava nada (pendente criado há 1h)", removed)
	}
	if _, err := Load(recentID); err != nil {
		t.Fatalf("manifesto pendente recente %q não deveria ter sido removido: %v", recentID, err)
	}
}

// TestSaveAutomaticallyPrunesExpiredUndoneManifests confirma que Save()
// aciona a poda de manifestos já desfeitos e expirados como efeito
// colateral — é assim que "undo --list" deixa de crescer sem limite sem
// exigir nenhum comando de manutenção manual.
func TestSaveAutomaticallyPrunesExpiredUndoneManifests(t *testing.T) {
	withTempConfigDir(t)

	expiredID := saveUndone(t, time.Now().Add(-(PruneUndoneAfter + time.Hour)))

	// A próxima gravação real (ex: um novo organize-pdf) é quem dispara a
	// poda, usando time.Now() internamente.
	if _, _, err := Save(sampleManifest()); err != nil {
		t.Fatalf("Save() erro inesperado: %v", err)
	}

	if _, err := Load(expiredID); err == nil {
		t.Fatalf("manifesto expirado %q deveria ter sido removido pela poda automática de Save()", expiredID)
	}

	headers, _, err := List()
	if err != nil {
		t.Fatalf("List() erro inesperado: %v", err)
	}
	if len(headers) != 1 {
		t.Fatalf("List() devolveu %d cabeçalhos após a poda, esperava 1 (só o recém-gravado)", len(headers))
	}
}

// TestSaveReturnsPrunedPendingIDs confirma o contrato usado por organize-pdf
// para avisar o usuário: quando a poda automática disparada por Save()
// remove um manifesto PENDENTE (não um já desfeito), o ID dele volta em
// prunedPending — nunca em silêncio.
func TestSaveReturnsPrunedPendingIDs(t *testing.T) {
	withTempConfigDir(t)

	now := time.Now()
	// saveUndone() primeiro, de propósito: ela chama Save() por dentro, o
	// que já dispara a poda automática de Save() com os limiares REAIS.
	// Se o manifesto pendente expirado já existisse no disco nesse
	// instante, essa poda "incidental" (de um Save() que não é o testado)
	// já o removeria antes da asserção abaixo — writeManifestRaw (dentro
	// de savePendingWithAge) só entra DEPOIS, sem esse efeito colateral.
	expiredUndoneID := saveUndone(t, now.Add(-(PruneUndoneAfter + time.Hour)))
	expiredPendingID := savePendingWithAge(t, now, PrunePendingAfter+24*time.Hour)

	_, prunedPending, err := Save(sampleManifest())
	if err != nil {
		t.Fatalf("Save() erro inesperado: %v", err)
	}

	if len(prunedPending) != 1 || prunedPending[0] != expiredPendingID {
		t.Fatalf("Save() prunedPending = %v, esperava só [%q]", prunedPending, expiredPendingID)
	}
	if containsID(prunedPending, expiredUndoneID) {
		t.Fatalf("Save() prunedPending não deveria incluir o manifesto já desfeito %q: %v", expiredUndoneID, prunedPending)
	}

	// Os dois foram de fato removidos do disco (undoneAfter e pendingAfter
	// continuam sendo aplicados juntos), só a categoria reportada difere.
	if _, err := Load(expiredPendingID); err == nil {
		t.Fatalf("manifesto pendente expirado %q deveria ter sido removido", expiredPendingID)
	}
	if _, err := Load(expiredUndoneID); err == nil {
		t.Fatalf("manifesto desfeito expirado %q deveria ter sido removido", expiredUndoneID)
	}
}

func containsID(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// TestPrunePlanDoesNotTouchDisk garante que PrunePlan calcula o que SERIA
// removido sem apagar nada — usado por "undo --prune" para mostrar e pedir
// confirmação antes de uma poda manual de verdade.
func TestPrunePlanDoesNotTouchDisk(t *testing.T) {
	withTempConfigDir(t)

	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local)
	// saveUndone() primeiro, pelo mesmo motivo do comentário em
	// TestSaveReturnsPrunedPendingIDs: ela chama Save() por dentro, que já
	// dispara a poda automática de Save() com os limiares REAIS — se o
	// manifesto pendente expirado já existisse, seria varrido ali, antes
	// mesmo de PrunePlan (a função sob teste) ser chamada.
	expiredUndoneID := saveUndone(t, now.Add(-31*24*time.Hour))
	expiredPendingID := savePendingWithAge(t, now, testPendingRetention+24*time.Hour)

	pending, undone, err := PrunePlan(now, testUndoneRetention, testPendingRetention)
	if err != nil {
		t.Fatalf("PrunePlan() erro inesperado: %v", err)
	}
	if len(pending) != 1 || pending[0] != expiredPendingID {
		t.Fatalf("PrunePlan() pending = %v, esperava [%q]", pending, expiredPendingID)
	}
	if len(undone) != 1 || undone[0] != expiredUndoneID {
		t.Fatalf("PrunePlan() undone = %v, esperava [%q]", undone, expiredUndoneID)
	}

	// Nada deveria ter sido removido de verdade.
	if _, err := Load(expiredPendingID); err != nil {
		t.Fatalf("PrunePlan() não deveria ter removido %q do disco: %v", expiredPendingID, err)
	}
	if _, err := Load(expiredUndoneID); err != nil {
		t.Fatalf("PrunePlan() não deveria ter removido %q do disco: %v", expiredUndoneID, err)
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
