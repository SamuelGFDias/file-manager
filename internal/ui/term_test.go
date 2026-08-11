package ui

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// TestIsOutputTerminalFalseForFile prova a metade "não-terminal" de
// IsOutputTerminal: com os.Stdout substituído por um arquivo comum
// (os.CreateTemp), isatty precisa devolver false — exatamente o que
// acontece quando alguém roda "file-manager --version > arquivo". É essa
// checagem que decide se o aviso de atualização disponível é impresso
// (ver printVersion, em internal/app/root.go): errar este caso faria um
// redirecionamento simples ganhar uma linha extra que hoje não existe.
func TestIsOutputTerminalFalseForFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "stdout-*.txt")
	if err != nil {
		t.Fatalf("os.CreateTemp: %v", err)
	}
	defer f.Close()

	original := os.Stdout
	os.Stdout = f
	defer func() { os.Stdout = original }()

	if IsOutputTerminal() {
		t.Errorf("IsOutputTerminal() = true com os.Stdout apontando para um arquivo comum, esperava false")
	}
}

// captureStdoutAndStderr redireciona os.Stdout e os.Stderr durante fn e
// devolve o que foi escrito em cada um.
func captureStdoutAndStderr(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() (stdout): %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() (stderr): %v", err)
	}

	originalOut, originalErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	defer func() { os.Stdout, os.Stderr = originalOut, originalErr }()

	fn()

	_ = outW.Close()
	_ = errW.Close()

	var outBuf, errBuf bytes.Buffer
	_, _ = io.Copy(&outBuf, outR)
	_, _ = io.Copy(&errBuf, errR)

	return outBuf.String(), errBuf.String()
}

// TestWarnStderrfAndInfoStderrfWriteToStderr prova que as duas variantes
// usadas para o aviso de atualização ao consultar a versão (ver
// internal/app.printVersion) escrevem em stderr, e nunca em stdout —
// diferente de Warnf/Infof (usadas pelo menu interativo e por outros
// comandos), que escrevem em stdout de propósito, porque ali não existe o
// requisito de "capturar só o stdout devolve exatamente a versão" que
// "--version"/"version" precisam preservar.
func TestWarnStderrfAndInfoStderrfWriteToStderr(t *testing.T) {
	stdout, stderr := captureStdoutAndStderr(t, func() {
		WarnStderrf("aviso de teste %d", 1)
		InfoStderrf("info de teste %d", 2)
	})

	if stdout != "" {
		t.Errorf("stdout = %q, esperava vazio — WarnStderrf/InfoStderrf não podem escrever em stdout", stdout)
	}
	if !strings.Contains(stderr, "aviso de teste 1") {
		t.Errorf("stderr não contém a mensagem de WarnStderrf: %q", stderr)
	}
	if !strings.Contains(stderr, "info de teste 2") {
		t.Errorf("stderr não contém a mensagem de InfoStderrf: %q", stderr)
	}
}
