//go:build e2e && linux

package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestNaoInterativoSemTTY confirma o outro lado da moeda do menu
// interativo: sem terminal (stdin ligado a /dev/null, como acontece quando
// o binário roda de um script, de um cron ou com stdin redirecionado), o
// programa deve falhar com uma mensagem clara e um código de saída
// diferente de zero, em vez de travar esperando input que nunca chega.
//
// Ao contrário dos demais testes deste pacote, este não usa
// internal/testcli/pty — o próprio propósito do teste é a AUSÊNCIA de um
// terminal, então um exec.Command comum (sem pty nenhum) é o cenário
// correto a reproduzir.
func TestNaoInterativoSemTTY(t *testing.T) {
	cmd := exec.Command(binPath)
	// Stdin nil conecta o processo a /dev/null (comportamento padrão do
	// os/exec) — sem terminal, sem pipe, exatamente a ausência de TTY que
	// este teste precisa reproduzir.
	cmd.Stdin = nil

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), isolatedEnv(t)...)

	err := cmd.Run()
	if err == nil {
		t.Fatalf(
			"processo saiu com código 0; esperava erro por ausência de terminal interativo.\nstdout:\n%s\nstderr:\n%s",
			stdout.String(), stderr.String(),
		)
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("erro inesperado ao rodar o processo: %v", err)
	}
	if exitErr.ExitCode() == 0 {
		t.Fatalf("código de saída = 0, esperava diferente de zero")
	}

	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, "terminal interativo") {
		t.Fatalf(
			"mensagem de erro não menciona a falta de terminal interativo (esperava conter \"terminal interativo\").\n"+
				"stdout:\n%s\nstderr:\n%s",
			stdout.String(), stderr.String(),
		)
	}
}
