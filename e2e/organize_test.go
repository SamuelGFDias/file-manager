//go:build e2e && linux

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamuelGFDias/file-manager/internal/testcli"
)

// enterOrganizeConfigureNow navega do menu principal até o início do fluxo
// de configuração de organize-pdf: seleciona "Organizar PDFs" (filtrando
// pelo texto em vez de contar "Down"s, para não depender da posição da
// ferramenta na lista) e responde "Configurar agora" (opção padrão) à
// pergunta sobre usar um perfil salvo. Devolve com o prompt de pasta de
// origem já na tela, mostrando startDir (o diretório de trabalho do
// processo) como diretório atual.
//
// Espera pela linha "Diretório atual: <startDir>", não pelo texto genérico
// "PASTA DE ORIGEM": esse texto fixo se repete em toda navegação do
// seletor de pastas, então usá-lo como alvo de Expect em um fluxo que passa
// pelo seletor mais de uma vez faria Expect ler conteúdo antigo do buffer
// acumulado e devolver na hora, sem esperar o redesenho de verdade — na
// prática destruindo a sincronização entre teste e programa (foi
// exatamente esse desenho que causou uma tecla enviada cedo demais e
// perdida na primeira versão deste teste). "Diretório atual: <startDir>" só
// existe, pela primeira vez, quando esse prompt específico realmente
// aparece.
func enterOrganizeConfigureNow(t *testing.T, sess *testcli.Session, startDir string) {
	t.Helper()

	sess.Expect("Organizar PDFs", defaultTimeout)
	sess.Send("Organizar PDFs")
	sess.Enter()

	sess.Expect("Como deseja configurar a organização?", defaultTimeout)
	sess.Enter() // "Configurar agora" é a opção padrão (primeiro item)

	sess.Expect("Diretório atual: "+startDir, defaultTimeout)
}

// TestOrganizePastaVaziaAvisaAntesDeCalibrar mira o defeito nº 3: antes da
// correção, uma pasta de origem sem PDFs só era percebida no final, depois
// de o usuário percorrer toda a calibração de regex (níveis, nome do
// arquivo, teste) — "0 de 0 arquivos organizados" era a primeira pista.
// pickInputDir agora conta os PDFs no ato da seleção e barra o avanço
// imediatamente. Este teste prova isso: seleciona uma pasta vazia como
// origem e confirma que o aviso aparece SEM que nenhuma pergunta de
// calibração (etapas 2 em diante) jamais tenha aparecido na tela.
func TestOrganizePastaVaziaAvisaAntesDeCalibrar(t *testing.T) {
	emptyDir := t.TempDir() // deliberadamente sem nenhum PDF

	sess := startBin(t, emptyDir)
	defer sess.Close()

	enterOrganizeConfigureNow(t, sess, emptyDir)

	// O processo foi iniciado com Dir=emptyDir, então o prompt de origem já
	// abre nela; "[ Selecionar esta pasta ]" é a opção padrão.
	sess.Enter()

	sess.Expect("não contém nenhum arquivo PDF", defaultTimeout)

	// A prova de que o aviso veio ANTES de qualquer pergunta de calibração:
	// nenhuma das etapas seguintes (2: pasta de destino, 4: níveis, 5: nome
	// do arquivo) pode ter aparecido em algum momento da execução até aqui.
	screen := sess.Screen()
	for _, naoEsperado := range []string{
		"Passo 2 de 7",
		"Passo 4 de 7",
		"Passo 5 de 7",
		"Adicionar um nível de pasta",
		"Renomear os arquivos com base no conteúdo",
	} {
		if strings.Contains(screen, naoEsperado) {
			t.Fatalf(
				"encontrou %q na tela antes mesmo de sair da seleção da pasta de origem vazia — "+
					"o aviso de pasta vazia deveria bloquear o avanço.\n--- tela capturada ---\n%s",
				naoEsperado, screen,
			)
		}
	}

	// Encerra o laço de forma limpa: escolhe "Cancelar" em vez de tentar de
	// novo.
	sess.Expect("O que deseja fazer?", defaultTimeout)
	sess.Down()
	sess.Enter()
}

// TestOrganizeSeletorDeDestinoComecaNaPastaDeOrigem mira o defeito nº 2:
// antes da correção, depois de escolher a pasta de origem, o seletor de
// pasta de destino reabria em "." — o diretório de trabalho do processo,
// na prática a pasta onde o executável foi deixado — em vez de continuar a
// partir da pasta de origem recém-selecionada. O usuário via uma listagem
// que não reconhecia e achava que a seleção anterior não tinha funcionado.
//
// O processo é iniciado em processDir (vazio, propositalmente diferente da
// pasta de origem), o que reproduziria o sintoma do defeito se ele ainda
// existisse: se o seletor de destino reabrisse em processDir, este teste
// pegaria isso. A navegação até a pasta de origem (sourceDir, uma pasta
// irmã de processDir) usa o filtro por texto do survey.Select em vez de
// contar setas, para não depender da ordem/nomes exatos das entradas de
// diretório.
func TestOrganizeSeletorDeDestinoComecaNaPastaDeOrigem(t *testing.T) {
	root := t.TempDir()
	processDir := filepath.Join(root, "processo")
	sourceDir := filepath.Join(root, "origem")
	for _, d := range []string{processDir, sourceDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("criar %s: %v", d, err)
		}
	}
	writeTestPDF(t, sourceDir, "fatura.pdf", []string{"conteudo de teste"})

	sess := startBin(t, processDir)
	defer sess.Close()

	enterOrganizeConfigureNow(t, sess, processDir)

	// Estamos em processDir (vazio): a única forma de avançar é voltar ao
	// diretório pai (root) e entrar em "origem/". Cada passo espera pela
	// linha "Diretório atual: <dir esperado>" — um marcador que só existe,
	// pela primeira vez, quando aquele diretório específico é de fato
	// mostrado — em vez do texto fixo "PASTA DE ORIGEM", que se repete a
	// cada nível de navegação e já apareceria no buffer acumulado antes da
	// hora (ver comentário de enterOrganizeConfigureNow).
	sess.Send("voltar")
	sess.Enter()
	// Não usamos "Diretório atual: "+root aqui: como root é prefixo de
	// processDir, essa string já teria aparecido (como prefixo de
	// "Diretório atual: "+processDir) antes mesmo do "voltar" ser enviado —
	// o mesmo problema de marcador "requentado" que estamos evitando em
	// outros pontos deste teste. "origem/" só aparece pela primeira vez
	// quando a listagem de root (que tem "origem/" e "processo/" como
	// subpastas) é de fato mostrada.
	sess.Expect("origem/", defaultTimeout)

	sess.Send("origem")
	sess.Enter()
	sess.Expect("Diretório atual: "+sourceDir, defaultTimeout)

	// Agora dentro de sourceDir: seleciona esta pasta como origem (opção
	// padrão).
	sess.Enter()

	sess.Expect("encontrados na pasta de origem", defaultTimeout)
	sess.Expect("Selecione a PASTA DE DESTINO", defaultTimeout)

	// A checagem de ausência importa aqui: o histórico completo da tela
	// contém "Diretório atual: <processDir>" várias vezes — legitimamente,
	// da navegação inicial dentro de processDir, antes de sair dele. Isso
	// não é o defeito. O que importa é o que aparece DEPOIS do prompt de
	// destino ser exibido: isolamos esse trecho para não confundir o
	// histórico legítimo com uma regressão.
	screen := sess.Screen()
	idx := strings.LastIndex(screen, "Selecione a PASTA DE DESTINO")
	if idx < 0 {
		t.Fatalf("prompt de destino não encontrado na tela capturada.\n--- tela ---\n%s", screen)
	}
	afterPrompt := screen[idx:]

	wantLine := "Diretório atual: " + sourceDir
	if !strings.Contains(afterPrompt, wantLine) {
		t.Fatalf(
			"o seletor de destino não mostrou a pasta de origem (%s) como diretório atual.\n"+
				"--- tela a partir do prompt de destino ---\n%s",
			sourceDir, afterPrompt,
		)
	}

	badLine := "Diretório atual: " + processDir
	if strings.Contains(afterPrompt, badLine) {
		t.Fatalf(
			"o seletor de destino reabriu no diretório do processo (%s) em vez de continuar a partir da "+
				"pasta de origem (%s) — defeito nº 2 reproduzido.\n--- tela a partir do prompt de destino ---\n%s",
			processDir, sourceDir, afterPrompt,
		)
	}

	sess.CtrlC()
}
