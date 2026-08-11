//go:build e2e && linux

package e2e

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SamuelGFDias/file-manager/internal/selfupdate"
	"github.com/SamuelGFDias/file-manager/internal/testcli"
)

// TestMenuMostraDescricaoApenasDaOpcaoSelecionada mira o comportamento
// explicitamente pedido pelo dono do projeto (Decisão 13 do AGENTS.md): a
// descrição de uma opção do menu só deve aparecer quando ela é a opção
// atualmente selecionada, nunca todas ao mesmo tempo. Um teste de unidade
// não pega isso — o template do survey é código de terceiro adaptado, e a
// única forma de saber se a adaptação continua correta é olhar o que
// aparece na tela de verdade.
func TestMenuMostraDescricaoApenasDaOpcaoSelecionada(t *testing.T) {
	sess := startBin(t, t.TempDir())
	defer sess.Close()

	// "Unir PDFs" é a primeira ferramenta (internal/app/registry.go) e
	// começa selecionada por padrão.
	sess.Expect("Unir PDFs", defaultTimeout)
	sess.Expect("Une vários arquivos PDF", defaultTimeout)
	sess.NotExpect("Separa um PDF em vários arquivos", 500*time.Millisecond)

	sess.Down()

	sess.Expect("Separa um PDF em vários arquivos", defaultTimeout)

	// A checagem de ausência aqui não pode usar NotExpect sobre a tela
	// inteira: o histórico acumulado ainda contém, legitimamente, a
	// descrição de "Unir PDFs" do primeiro redesenho (antes do Down). Isso
	// não seria o defeito — o que importa é se ela reaparece no redesenho
	// MAIS RECENTE, depois do Down. Isolamos esse trecho pelo marcador que
	// cada redesenho completo do survey.Select reimprime.
	screen := sess.Screen()
	const marker = "o que você deseja fazer?"
	idx := strings.LastIndex(screen, marker)
	if idx < 0 {
		t.Fatalf("marcador do prompt do menu não encontrado na tela capturada.\n--- tela ---\n%s", screen)
	}
	lastRender := screen[idx:]
	if strings.Contains(lastRender, "Une vários arquivos PDF") {
		t.Fatalf(
			"a descrição de \"Unir PDFs\" ainda aparece no redesenho mais recente do menu, depois de mover "+
				"a seleção para \"Separar PDFs\" — deveria mostrar só a descrição da opção selecionada.\n"+
				"--- redesenho mais recente ---\n%s",
			lastRender,
		)
	}
}

// TestMenuAvisaVersaoNova mira o defeito nº 1: a checagem de atualização
// roda em segundo plano (~250ms) e, sem WaitNotice, o menu já tinha
// entregado o terminal ao survey.Select antes do resultado chegar — o
// aviso praticamente nunca aparecia na primeira abertura. Compila um
// binário com uma versão propositalmente antiga (v0.0.1) e confirma que o
// aviso aparece já na primeira renderização do menu.
//
// Depende da API real do GitHub (o mesmo endpoint que o binário publicado
// consulta) — não há como simular isso sem alterar código de produção
// (apiBaseURL só é substituível de dentro do próprio pacote selfupdate, em
// testes que o compilam junto). Por isso o teste primeiro faz a mesma
// consulta que o Checker faria e pula com t.Skip, motivo explícito, se ela
// falhar por qualquer razão (sem rede, limite de requisições, repositório
// sem release publicado) — nunca falha por causa do ambiente.
func TestMenuAvisaVersaoNova(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	release, err := selfupdate.LatestRelease(ctx, selfupdate.DefaultRepo)
	if err != nil {
		t.Skipf(
			"sem acesso à API do GitHub (%s) neste ambiente (%v); pulando teste que depende de uma checagem "+
				"real de atualização — não é um defeito do harness nem do binário",
			selfupdate.DefaultRepo, err,
		)
	}

	const oldVersion = "v0.0.1"
	if cmp, cmpErr := selfupdate.CompareVersions(oldVersion, release.TagName); cmpErr != nil || cmp >= 0 {
		t.Skipf(
			"a última versão publicada de %s é %q, que não é mais nova que %s; não há como forçar o aviso "+
				"de atualização neste repositório sem alterar código de produção",
			selfupdate.DefaultRepo, release.TagName, oldVersion,
		)
	}

	oldBin := filepath.Join(t.TempDir(), "file-manager-v0.0.1")
	if err := buildBinary(oldBin, "-X main.version="+oldVersion); err != nil {
		t.Fatalf("compilar binário de teste com versão %s: %v", oldVersion, err)
	}

	sess := testcli.Start(t, testcli.Options{
		Bin: oldBin,
		Dir: t.TempDir(),
		Env: isolatedEnv(t),
	})
	defer sess.Close()

	// O aviso precisa aparecer na PRIMEIRA renderização — WaitNotice paga
	// até 1,5s de espera antes de o menu abrir. Timeout generoso o
	// suficiente para cobrir isso mais a latência real de rede e o tempo de
	// start do processo, sem mascarar uma regressão que faça o aviso levar
	// muito mais tempo que isso para aparecer.
	sess.Expect("nova versão disponível: "+oldVersion, 8*time.Second)
	sess.Expect("update", 200*time.Millisecond)
}

// TestMenuSaiComOpcaoSair navega até "Sair" e confirma que o processo
// termina com código 0. "Sair" é sempre a última opção do menu
// (mainmenu/screen.go); em vez de contar quantos "Down" são necessários
// (o que quebraria se uma ferramenta nova fosse registrada), usamos o
// wrap-around do survey.Select: uma seta para cima a partir da primeira
// opção vai direto para a última.
func TestMenuSaiComOpcaoSair(t *testing.T) {
	sess := startBin(t, t.TempDir())
	defer sess.Close()

	sess.Expect("Sair", defaultTimeout)
	sess.Up()
	sess.Enter()

	code := sess.Wait(defaultTimeout)
	if code != 0 {
		t.Fatalf("código de saída = %d, esperava 0.\n--- tela capturada ---\n%s", code, sess.Screen())
	}
}
