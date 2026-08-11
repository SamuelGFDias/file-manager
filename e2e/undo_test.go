//go:build e2e && linux

package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// runCLI executa o binário sob teste diretamente (sem terminal virtual): o
// cenário deste arquivo não precisa de nenhum prompt interativo — tanto
// "organize-pdf --move" quanto "undo --last -y" recebem tudo via flag —,
// então um exec.Command comum basta, no mesmo espírito de
// TestNaoInterativoSemTTY (nontty_test.go). cmd.Stdin não é definido
// (nil), o que conecta o processo a /dev/null: sem terminal de verdade,
// exatamente o caminho não-interativo que este cenário exercita.
func runCLI(t *testing.T, dir string, args []string, extraEnv ...string) (stdout, stderr string) {
	t.Helper()

	cmd := exec.Command(binPath, args...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), isolatedEnv(t)...), extraEnv...)

	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		t.Fatalf(
			"%s %v falhou: %v\nstdout:\n%s\nstderr:\n%s",
			binPath, args, err, out.String(), errBuf.String(),
		)
	}

	return out.String(), errBuf.String()
}

// TestUndoMoveRestoresFilesToSource é o cenário ponta a ponta pedido:
// organiza uma pasta de PDFs com --move pela linha de comando, roda
// "undo --last -y" e confirma que os arquivos voltaram à pasta de origem
// original (e desapareceram do destino).
//
// "--last" é necessário porque, sem terminal interativo e sem --id, undo
// não tem como perguntar qual operação desfazer (ver resolveUndoManifest em
// internal/app/undo.go) — com um único manifesto registrado neste cenário,
// "a mais recente" é exatamente a que acabou de ser criada.
//
// As duas chamadas ao binário (organizar e desfazer) precisam enxergar o
// MESMO diretório de configuração — é lá que o manifesto fica gravado entre
// uma chamada e outra — então este teste fixa XDG_CONFIG_HOME uma única vez
// (configEnv) e o passa às duas, em vez de deixar isolatedEnv(t) gerar um
// valor novo (e portanto um histórico vazio) a cada chamada.
func TestUndoMoveRestoresFilesToSource(t *testing.T) {
	configEnv := "XDG_CONFIG_HOME=" + t.TempDir()

	inputDir := t.TempDir()
	outputDir := t.TempDir()

	names := []string{"nota-a.pdf", "nota-b.pdf"}
	for _, name := range names {
		writeTestPDF(t, inputDir, name, []string{"conteudo de teste"})
	}

	organizeOut, _ := runCLI(t, inputDir, []string{
		"organize-pdf",
		"--input", inputDir,
		"--output", outputDir,
		"--move",
	}, configEnv)
	t.Logf("organize-pdf --move:\n%s", organizeOut)

	// Sem --level nem --filename-regex, os arquivos vão direto (mantendo o
	// nome original) para outputDir — a pasta de origem deve esvaziar.
	remaining, err := os.ReadDir(inputDir)
	if err != nil {
		t.Fatalf("ler inputDir após organizar: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("esperava pasta de origem vazia após --move, restou: %v", remaining)
	}
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Fatalf("esperava %q em outputDir após organizar: %v", name, err)
		}
	}

	undoOut, _ := runCLI(t, inputDir, []string{"undo", "--last", "-y"}, configEnv)
	t.Logf("undo --last -y:\n%s", undoOut)

	for _, name := range names {
		if _, err := os.Stat(filepath.Join(inputDir, name)); err != nil {
			t.Fatalf("esperava %q de volta em inputDir após undo, obteve erro: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(outputDir, name)); !os.IsNotExist(err) {
			t.Fatalf("esperava %q removido de outputDir após undo, err=%v", name, err)
		}
	}
}
