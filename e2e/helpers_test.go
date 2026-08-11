//go:build e2e && linux

package e2e

import (
	"testing"
	"time"

	"github.com/SamuelGFDias/file-manager/internal/testcli"
)

// defaultTimeout é usado pela maioria dos Expect/ExpectAny deste pacote:
// folga generosa para um processo que abre pty, faz I/O de disco e
// desenha prompts via survey, sem deixar os testes lentos demais quando
// tudo corre bem (o polling do harness devolve assim que o texto aparece,
// não espera o timeout completo).
const defaultTimeout = 5 * time.Second

// isolatedEnv devolve as variáveis de ambiente para isolar uma sessão de
// teste do ambiente real da máquina:
//
//   - XDG_CONFIG_HOME aponta para um diretório temporário exclusivo do
//     teste, para que os.UserConfigDir() (usado por internal/config para
//     achar a pasta de perfis) nunca leia nem grave em
//     ~/.config/file-manager de verdade.
//   - PATH é sobrescrito para excluir qualquer instalação real do
//     Tesseract no PATH da máquina de desenvolvimento/CI. Isso é
//     necessário porque internal/ocr.NewTesseract() cai em
//     exec.LookPath("tesseract") sempre que TESSERACT_PATH está vazio ou
//     inválido — então só invalidar TESSERACT_PATH não bastaria para
//     desligar o OCR em uma máquina que já tem o tesseract instalado (como
//     esta, em /usr/bin/tesseract). Com PATH vazio de um binário
//     "tesseract" e TESSERACT_PATH também inválido, ocr.Available()
//     devolve false de forma confiável, e o fluxo interativo de
//     organize-pdf pula a pergunta "Usar OCR?" (só feita quando o
//     Tesseract está disponível) — o que mantém os testes que não são
//     sobre OCR determinísticos e rápidos, independente da máquina onde
//     rodam.
func isolatedEnv(t *testing.T) []string {
	t.Helper()
	return []string{
		"XDG_CONFIG_HOME=" + t.TempDir(),
		"TESSERACT_PATH=/nao-existe/tesseract",
		"PATH=/nao-existe/bin",
	}
}

// startBin inicia uma sessão do binário padrão (compilado uma vez em
// TestMain) dentro de dir, com o ambiente isolado de isolatedEnv mais
// qualquer extra informado. Registra o encerramento via t.Cleanup
// (indiretamente, dentro de testcli.Start).
func startBin(t *testing.T, dir string, extraEnv ...string) *testcli.Session {
	t.Helper()
	env := append(isolatedEnv(t), extraEnv...)
	return testcli.Start(t, testcli.Options{
		Bin: binPath,
		Dir: dir,
		Env: env,
	})
}
