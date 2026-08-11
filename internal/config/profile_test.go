package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
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

func TestExportProfileThenReadProfileFileRoundTrip(t *testing.T) {
	withTempConfigDir(t)

	in := sampleData{Label: "exportado", Count: 7, Tags: []string{"x", "y"}}
	if err := Save(testTool, "para-exportar", in); err != nil {
		t.Fatalf("Save falhou: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "subpasta", "perfil.yaml")
	if err := ExportProfile(testTool, "para-exportar", dest); err != nil {
		t.Fatalf("ExportProfile falhou: %v", err)
	}

	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("arquivo exportado não encontrado em %q: %v", dest, err)
	}

	imported, err := ReadProfileFile(dest)
	if err != nil {
		t.Fatalf("ReadProfileFile falhou: %v", err)
	}

	if imported.Tool != testTool {
		t.Errorf("Tool = %q, esperava %q", imported.Tool, testTool)
	}
	if imported.Name != "para-exportar" {
		t.Errorf("Name = %q, esperava %q", imported.Name, "para-exportar")
	}

	var out sampleData
	if err := imported.Node.Decode(&out); err != nil {
		t.Fatalf("erro ao decodificar Node: %v", err)
	}
	if out.Label != in.Label || out.Count != in.Count || len(out.Tags) != len(in.Tags) {
		t.Fatalf("round-trip incorreto: got %+v, want %+v", out, in)
	}
}

func TestReadProfileFileNonExistent(t *testing.T) {
	_, err := ReadProfileFile(filepath.Join(t.TempDir(), "nao-existe.yaml"))
	if err == nil {
		t.Fatalf("esperava erro para arquivo inexistente")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("esperava errors.Is(err, os.ErrNotExist), got %v", err)
	}
}

func TestReadProfileFileInvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalido.yaml")
	if err := os.WriteFile(path, []byte("isto: [não fecha"), 0o644); err != nil {
		t.Fatalf("falha ao preparar arquivo de teste: %v", err)
	}

	_, err := ReadProfileFile(path)
	if err == nil {
		t.Fatalf("esperava erro para YAML inválido")
	}
}

func TestReadProfileFileEmptyTool(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sem-tool.yaml")
	content := "name: algum-nome\ntool: \"\"\ndata:\n  label: x\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("falha ao preparar arquivo de teste: %v", err)
	}

	_, err := ReadProfileFile(path)
	if err == nil {
		t.Fatalf("esperava erro para campo \"tool\" vazio")
	}
	if !strings.Contains(err.Error(), "tool") {
		t.Errorf("erro deveria mencionar o campo \"tool\": %v", err)
	}
}

func TestReadProfileFileInvalidName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nome-invalido.yaml")
	content := "name: ../x\ntool: example-tool\ndata:\n  label: x\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("falha ao preparar arquivo de teste: %v", err)
	}

	_, err := ReadProfileFile(path)
	if err == nil {
		t.Fatalf("esperava erro para campo \"name\" inválido")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("erro deveria mencionar o campo \"name\": %v", err)
	}
}

func TestImportProfileFailsWhenExistsAndNotOverwrite(t *testing.T) {
	withTempConfigDir(t)

	if err := Save(testTool, "ja-existe", sampleData{Label: "original"}); err != nil {
		t.Fatalf("Save falhou: %v", err)
	}

	imported := ImportedProfile{
		Name: "ja-existe",
		Tool: testTool,
		Node: encodeNode(t, sampleData{Label: "novo"}),
	}

	err := ImportProfile(imported, "ja-existe", false)
	if err == nil {
		t.Fatalf("esperava erro ao importar sobre perfil existente sem overwrite")
	}

	var out sampleData
	if err := Load(testTool, "ja-existe", &out); err != nil {
		t.Fatalf("Load falhou: %v", err)
	}
	if out.Label != "original" {
		t.Fatalf("perfil original não deveria ter sido sobrescrito: got %+v", out)
	}
}

func TestImportProfileOverwritesAndPreservesCreatedAt(t *testing.T) {
	withTempConfigDir(t)

	if err := Save(testTool, "sobrescrever", sampleData{Label: "original"}); err != nil {
		t.Fatalf("Save falhou: %v", err)
	}
	original, err := LoadProfile(testTool, "sobrescrever")
	if err != nil {
		t.Fatalf("LoadProfile falhou: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	imported := ImportedProfile{
		Name: "sobrescrever",
		Tool: testTool,
		Node: encodeNode(t, sampleData{Label: "importado"}),
	}

	if err := ImportProfile(imported, "sobrescrever", true); err != nil {
		t.Fatalf("ImportProfile falhou: %v", err)
	}

	var out sampleData
	if err := Load(testTool, "sobrescrever", &out); err != nil {
		t.Fatalf("Load falhou: %v", err)
	}
	if out.Label != "importado" {
		t.Fatalf("esperava dados importados, got %+v", out)
	}

	after, err := LoadProfile(testTool, "sobrescrever")
	if err != nil {
		t.Fatalf("LoadProfile falhou: %v", err)
	}
	if !after.CreatedAt.Equal(original.CreatedAt) {
		t.Fatalf("CreatedAt deveria ser preservado: original %v, got %v", original.CreatedAt, after.CreatedAt)
	}
	if !after.UpdatedAt.After(original.UpdatedAt) {
		t.Fatalf("UpdatedAt deveria avançar: original %v, got %v", original.UpdatedAt, after.UpdatedAt)
	}
}

func TestImportProfileWithDifferentNameWritesUnderNewName(t *testing.T) {
	withTempConfigDir(t)

	imported := ImportedProfile{
		Name: "nome-do-arquivo",
		Tool: testTool,
		Node: encodeNode(t, sampleData{Label: "conteudo"}),
	}

	if err := ImportProfile(imported, "nome-novo", false); err != nil {
		t.Fatalf("ImportProfile falhou: %v", err)
	}

	existsOld, err := Exists(testTool, "nome-do-arquivo")
	if err != nil {
		t.Fatalf("Exists falhou: %v", err)
	}
	if existsOld {
		t.Fatalf("perfil não deveria ter sido gravado sob o nome original do arquivo")
	}

	existsNew, err := Exists(testTool, "nome-novo")
	if err != nil {
		t.Fatalf("Exists falhou: %v", err)
	}
	if !existsNew {
		t.Fatalf("perfil deveria ter sido gravado sob o novo nome")
	}
}

func TestDecodeErrorIsUserFriendlyAndWrapsOriginal(t *testing.T) {
	original := errors.New("cannot unmarshal !!str `abc` into []organizepdf.LevelSpec")

	err := DecodeError("organize-pdf", "/tmp/perfil.yaml", original)
	if err == nil {
		t.Fatalf("DecodeError não deveria devolver nil")
	}

	msg := err.Error()
	for _, want := range []string{"organize-pdf", "/tmp/perfil.yaml", "corrompido", "editado à mão", "versão diferente"} {
		if !strings.Contains(msg, want) {
			t.Errorf("mensagem deveria conter %q; got: %s", want, msg)
		}
	}

	if !errors.Is(err, original) {
		t.Fatalf("DecodeError deveria encapsular o erro original com %%w (errors.Is falhou)")
	}
}

// encodeNode codifica v num yaml.Node, para montar ImportedProfile.Node nos
// testes sem depender de um arquivo real em disco.
func encodeNode(t *testing.T, v any) yaml.Node {
	t.Helper()

	var node yaml.Node
	if err := node.Encode(v); err != nil {
		t.Fatalf("erro ao codificar yaml.Node: %v", err)
	}
	return node
}
