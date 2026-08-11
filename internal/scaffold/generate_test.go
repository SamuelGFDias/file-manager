package scaffold

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestDeriveNames_Valid(t *testing.T) {
	cases := []struct {
		in      string
		wantPkg string
		wantTyp string
	}{
		{in: "compress-pdf", wantPkg: "compresspdf", wantTyp: "CompressPdf"},
		{in: "ocr", wantPkg: "ocr", wantTyp: "Ocr"},
	}

	for _, c := range cases {
		got, err := DeriveNames(c.in)
		if err != nil {
			t.Fatalf("DeriveNames(%q) devolveu erro inesperado: %v", c.in, err)
		}
		if got.Kebab != c.in {
			t.Errorf("DeriveNames(%q).Kebab = %q, esperava %q", c.in, got.Kebab, c.in)
		}
		if got.Package != c.wantPkg {
			t.Errorf("DeriveNames(%q).Package = %q, esperava %q", c.in, got.Package, c.wantPkg)
		}
		if got.Type != c.wantTyp {
			t.Errorf("DeriveNames(%q).Type = %q, esperava %q", c.in, got.Type, c.wantTyp)
		}
	}
}

func TestDeriveNames_Invalid(t *testing.T) {
	invalid := []string{
		"",
		"Compress",
		"compress_pdf",
		"-x",
		"x-",
		"a--b",
		"1abc",
	}

	for _, in := range invalid {
		if _, err := DeriveNames(in); err == nil {
			t.Errorf("DeriveNames(%q) esperava erro, obteve nil", in)
		}
	}
}

func TestGenerate_CreatesExpectedFiles(t *testing.T) {
	root := t.TempDir()

	created, err := Generate(Options{Name: "compress-pdf", OutputRoot: root})
	if err != nil {
		t.Fatalf("Generate devolveu erro inesperado: %v", err)
	}

	dir := filepath.Join(root, "internal", "tools", "compresspdf")

	wantNames := []string{
		"command.go",
		"compresspdf_test.go",
		"options.go",
		"screen.go",
		"tool.go",
	}

	wantPaths := make([]string, 0, len(wantNames))
	for _, name := range wantNames {
		abs, err := filepath.Abs(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("erro ao resolver caminho absoluto esperado: %v", err)
		}
		wantPaths = append(wantPaths, abs)
	}
	sort.Strings(wantPaths)

	if len(created) != len(wantPaths) {
		t.Fatalf("Generate criou %d arquivos, esperava %d: %v", len(created), len(wantPaths), created)
	}
	for i := range wantPaths {
		if created[i] != wantPaths[i] {
			t.Errorf("caminho criado[%d] = %q, esperava %q", i, created[i], wantPaths[i])
		}
	}

	for _, p := range created {
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("arquivo %q não existe: %v", p, err)
			continue
		}
		if info.IsDir() {
			t.Errorf("%q esperava ser arquivo, é diretório", p)
		}
	}
}

func TestGenerate_ExistingDirWithoutForceFails(t *testing.T) {
	root := t.TempDir()
	opts := Options{Name: "compress-pdf", OutputRoot: root}

	if _, err := Generate(opts); err != nil {
		t.Fatalf("primeira chamada a Generate devolveu erro inesperado: %v", err)
	}

	if _, err := Generate(opts); err == nil {
		t.Fatal("segunda chamada a Generate sem Force esperava erro, obteve nil")
	}
}

func TestGenerate_ExistingDirWithForceSucceeds(t *testing.T) {
	root := t.TempDir()
	opts := Options{Name: "compress-pdf", OutputRoot: root}

	if _, err := Generate(opts); err != nil {
		t.Fatalf("primeira chamada a Generate devolveu erro inesperado: %v", err)
	}

	opts.Force = true
	if _, err := Generate(opts); err != nil {
		t.Fatalf("segunda chamada a Generate com Force devolveu erro inesperado: %v", err)
	}
}

func TestGenerate_OutputIsValidGo(t *testing.T) {
	root := t.TempDir()

	created, err := Generate(Options{Name: "compress-pdf", OutputRoot: root})
	if err != nil {
		t.Fatalf("Generate devolveu erro inesperado: %v", err)
	}

	for _, path := range created {
		fset := token.NewFileSet()
		if _, err := parser.ParseFile(fset, path, nil, parser.AllErrors); err != nil {
			t.Errorf("arquivo gerado %q não é Go sintaticamente válido: %v", path, err)
		}
	}
}

func TestGenerate_OutputContainsExpectedPackageAndUse(t *testing.T) {
	root := t.TempDir()

	created, err := Generate(Options{Name: "compress-pdf", OutputRoot: root})
	if err != nil {
		t.Fatalf("Generate devolveu erro inesperado: %v", err)
	}

	var toolGo, commandGo string
	for _, path := range created {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("erro ao ler %q: %v", path, err)
		}

		if !strings.Contains(string(content), "package compresspdf") {
			t.Errorf("arquivo %q não contém \"package compresspdf\"", path)
		}

		switch filepath.Base(path) {
		case "tool.go":
			toolGo = string(content)
		case "command.go":
			commandGo = string(content)
		}
	}

	if toolGo == "" {
		t.Fatal("tool.go não foi encontrado entre os arquivos gerados")
	}
	if commandGo == "" {
		t.Fatal("command.go não foi encontrado entre os arquivos gerados")
	}
	if !strings.Contains(commandGo, `Use: "compress-pdf"`) {
		t.Errorf("command.go não contém `Use: \"compress-pdf\"`: %s", commandGo)
	}
}
