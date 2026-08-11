package config

import (
	"errors"
	"os"
	"testing"
	"time"
)

// withTempConfigDir substitui userConfigDir por um diretório temporário
// durante o teste, restaurando o valor original ao final.
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

type sampleData struct {
	Label string   `yaml:"label"`
	Count int      `yaml:"count"`
	Tags  []string `yaml:"tags"`
}

const testTool = "example-tool"

func TestSaveLoadRoundTrip(t *testing.T) {
	withTempConfigDir(t)

	in := sampleData{
		Label: "meu-perfil",
		Count: 42,
		Tags:  []string{"a", "b", "c"},
	}

	if err := Save(testTool, "roundtrip", in); err != nil {
		t.Fatalf("Save falhou: %v", err)
	}

	var out sampleData
	if err := Load(testTool, "roundtrip", &out); err != nil {
		t.Fatalf("Load falhou: %v", err)
	}

	if out.Label != in.Label || out.Count != in.Count || len(out.Tags) != len(in.Tags) {
		t.Fatalf("round-trip incorreto: got %+v, want %+v", out, in)
	}
	for i := range in.Tags {
		if out.Tags[i] != in.Tags[i] {
			t.Fatalf("tags[%d] incorreta: got %q, want %q", i, out.Tags[i], in.Tags[i])
		}
	}
}

func TestListEmptyWhenNoProfiles(t *testing.T) {
	withTempConfigDir(t)

	names, err := List(testTool)
	if err != nil {
		t.Fatalf("List não deveria retornar erro quando o diretório não existe: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("esperava slice vazio, got %v", names)
	}
}

func TestListReturnsSortedNames(t *testing.T) {
	withTempConfigDir(t)

	for _, name := range []string{"zebra", "abacaxi", "melancia"} {
		if err := Save(testTool, name, sampleData{Label: name}); err != nil {
			t.Fatalf("Save(%q) falhou: %v", name, err)
		}
	}

	names, err := List(testTool)
	if err != nil {
		t.Fatalf("List falhou: %v", err)
	}

	want := []string{"abacaxi", "melancia", "zebra"}
	if len(names) != len(want) {
		t.Fatalf("esperava %v, got %v", want, names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("esperava %v, got %v", want, names)
		}
	}
}

func TestExists(t *testing.T) {
	withTempConfigDir(t)

	ok, err := Exists(testTool, "algum-perfil")
	if err != nil {
		t.Fatalf("Exists falhou: %v", err)
	}
	if ok {
		t.Fatalf("Exists deveria retornar false antes do Save")
	}

	if err := Save(testTool, "algum-perfil", sampleData{Label: "x"}); err != nil {
		t.Fatalf("Save falhou: %v", err)
	}

	ok, err = Exists(testTool, "algum-perfil")
	if err != nil {
		t.Fatalf("Exists falhou: %v", err)
	}
	if !ok {
		t.Fatalf("Exists deveria retornar true depois do Save")
	}
}

func TestSaveTwicePreservesCreatedAt(t *testing.T) {
	withTempConfigDir(t)

	if err := Save(testTool, "persistente", sampleData{Label: "v1"}); err != nil {
		t.Fatalf("Save (1) falhou: %v", err)
	}

	first, err := LoadProfile(testTool, "persistente")
	if err != nil {
		t.Fatalf("LoadProfile (1) falhou: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	if err := Save(testTool, "persistente", sampleData{Label: "v2"}); err != nil {
		t.Fatalf("Save (2) falhou: %v", err)
	}

	second, err := LoadProfile(testTool, "persistente")
	if err != nil {
		t.Fatalf("LoadProfile (2) falhou: %v", err)
	}

	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("CreatedAt deveria ser preservado: original %v, got %v", first.CreatedAt, second.CreatedAt)
	}
	if second.UpdatedAt.Before(second.CreatedAt) {
		t.Fatalf("UpdatedAt (%v) não pode ser anterior a CreatedAt (%v)", second.UpdatedAt, second.CreatedAt)
	}
	if !second.UpdatedAt.After(first.UpdatedAt) {
		t.Fatalf("UpdatedAt deveria avançar entre os dois Save: primeiro %v, segundo %v", first.UpdatedAt, second.UpdatedAt)
	}
}

func TestLoadNonExistentProfile(t *testing.T) {
	withTempConfigDir(t)

	var out sampleData
	err := Load(testTool, "nao-existe", &out)
	if err == nil {
		t.Fatalf("esperava erro ao carregar perfil inexistente")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("esperava errors.Is(err, os.ErrNotExist), got %v", err)
	}
}

func TestDelete(t *testing.T) {
	withTempConfigDir(t)

	if err := Save(testTool, "descartavel", sampleData{Label: "x"}); err != nil {
		t.Fatalf("Save falhou: %v", err)
	}

	if err := Delete(testTool, "descartavel"); err != nil {
		t.Fatalf("Delete falhou: %v", err)
	}

	ok, err := Exists(testTool, "descartavel")
	if err != nil {
		t.Fatalf("Exists falhou: %v", err)
	}
	if ok {
		t.Fatalf("perfil deveria ter sido removido")
	}
}

func TestDeleteNonExistentProfile(t *testing.T) {
	withTempConfigDir(t)

	err := Delete(testTool, "nao-existe")
	if err == nil {
		t.Fatalf("esperava erro ao deletar perfil inexistente")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("esperava errors.Is(err, os.ErrNotExist), got %v", err)
	}
}

func TestValidateNameRejectsInvalid(t *testing.T) {
	invalid := []string{
		"",
		"a/b",
		`a\b`,
		"..",
		"../x",
		"nome com espaço",
		"nome!",
	}

	for _, name := range invalid {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) deveria falhar", name)
		}
	}
}

func TestValidateNameAcceptsValid(t *testing.T) {
	valid := []string{
		"separar-nf",
		"perfil_1",
		"a.b",
	}

	for _, name := range valid {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) não deveria falhar: %v", name, err)
		}
	}
}

func TestLoadWithMaliciousNameFailsValidation(t *testing.T) {
	dir := withTempConfigDir(t)

	// Cria um arquivo fora do diretório de perfis para garantir que, caso a
	// validação falhasse em impedir o acesso, o teste detectaria uma leitura
	// indevida.
	outsideFile := dir + "/passwd-like.yaml"
	if err := os.WriteFile(outsideFile, []byte("name: leaked\n"), 0o644); err != nil {
		t.Fatalf("falha ao preparar arquivo de teste: %v", err)
	}

	var out sampleData
	err := Load(testTool, "../../passwd-like", &out)
	if err == nil {
		t.Fatalf("esperava erro de validação para nome malicioso")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("erro deveria ser de validação, não de arquivo inexistente: %v", err)
	}
}
