//go:build e2e && linux

// Package e2e contém os testes ponta a ponta do CLI file-manager: cada um
// abre o binário real dentro de um pseudo-terminal (via
// internal/testcli), envia teclas como um usuário faria e verifica o que
// apareceu na tela.
//
// Por quê: o projeto tem 16 pacotes de teste verdes com -race e, mesmo
// assim, três defeitos sérios chegaram ao usuário — todos na costura entre
// peças corretas isoladamente (redesenho de tela, ordem de navegação entre
// seletores), nunca dentro de uma função pura testável isoladamente. Nenhum
// teste que chama funções Go diretamente exercitaria esses defeitos; só
// abrir o processo de verdade, com um terminal de verdade, pega esse tipo
// de problema.
//
// Ficam sob a tag de build "e2e" (além de "linux", exigida pelo harness)
// para não rodar no "go test ./..." do dia a dia: são lentos (cada cenário
// inicia um processo e navega por prompts reais) e dependem de recursos
// específicos de Linux (/dev/ptmx). Rode com "make e2e".
package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// modulePath é o caminho de import do módulo, usado para compilar o
// binário sob teste a partir do código-fonte deste checkout (nunca um
// binário publicado baixado de outro lugar).
const modulePath = "github.com/SamuelGFDias/file-manager/cmd/file-manager"

// binPath é o caminho do binário compilado uma única vez em TestMain e
// reaproveitado por todos os testes deste pacote que não precisam de uma
// versão especial (ex: TestMenuAvisaVersaoNova compila a sua própria, com
// -ldflags -X main.version=..., porque precisa de um valor de versão
// específico).
var binPath string

// TestMain compila o binário file-manager uma única vez, para um diretório
// temporário fora da árvore do repositório, antes de rodar qualquer teste
// deste pacote. Compilar uma vez em vez de em cada teste economiza,
// principalmente, o tempo do build do pdfcpu (dependência pesada).
func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "file-manager-e2e-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: erro ao criar diretório temporário para o binário: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	binPath = filepath.Join(tmpDir, "file-manager")
	if err := buildBinary(binPath, ""); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: erro ao compilar o binário sob teste: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// buildBinary compila o binário file-manager (a partir do código-fonte
// deste checkout, nunca um artefato publicado) para outPath.
// versionLdflag, quando não-vazio, é passado como -ldflags adicional (ex:
// "-X main.version=v0.0.1", usado por TestMenuAvisaVersaoNova para simular
// uma versão antiga e forçar o aviso de atualização a aparecer).
func buildBinary(outPath, versionLdflag string) error {
	args := []string{"build", "-o", outPath}
	if versionLdflag != "" {
		args = append(args, "-ldflags", versionLdflag)
	}
	args = append(args, modulePath)

	cmd := exec.Command("go", args...)
	// CGO_ENABLED=0 replica o build de produção (ver Makefile) — o binário
	// publicado é sempre Go puro, sem toolchain C.
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go %v: %w\n%s", args, err, out)
	}
	return nil
}
