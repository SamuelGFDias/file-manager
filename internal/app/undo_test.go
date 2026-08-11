package app

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SamuelGFDias/file-manager/internal/history"
)

// captureStdout redireciona os.Stdout durante fn e devolve tudo que foi
// escrito nele. Necessário porque ui.Infof/ui.Warnf (usados por
// printUndoList) escrevem direto em os.Stdout via fmt.Printf, não por um
// io.Writer injetável (cobra.Command.SetOut não cobre esse caminho).
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(): %v", err)
	}

	original := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = original }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("fechar pipe: %v", err)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("ler pipe: %v", err)
	}
	return buf.String()
}

// saveNManifests grava n manifestos pendentes, todos recentes (CreatedAt
// escalonado em minutos a partir de agora, sempre no passado, sempre bem
// dentro do prazo de retenção de pendentes) — recente o bastante para não
// serem varridos pela poda automática que history.Save dispara a cada
// gravação. InputDir de cada um recebe um sufixo numérico único, para os
// testes conseguirem contar/distinguir linhas na saída.
func saveNManifests(t *testing.T, n int) {
	t.Helper()

	now := time.Now()
	for i := 0; i < n; i++ {
		m := history.Manifest{
			Tool:      "organize-pdf",
			CreatedAt: now.Add(-time.Duration(n-i) * time.Minute),
			InputDir:  fmt.Sprintf("/tmp/origem-%03d", i),
			OutputDir: fmt.Sprintf("/tmp/destino-%03d", i),
			Action:    history.ActionCopy,
		}
		if _, _, err := history.Save(m); err != nil {
			t.Fatalf("history.Save(%d): %v", i, err)
		}
	}
}

// TestPrintUndoListLimitsToDefaultWithFooter prova o limite padrão de
// exibição de "undo --list": com mais manifestos que
// history.ListDisplayLimit, só os mais recentes aparecem, e um rodapé
// avisa quantos ficaram de fora — um histórico grande despejado sem limite
// na tela é inutilizável.
func TestPrintUndoListLimitsToDefaultWithFooter(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	total := history.ListDisplayLimit + 5
	saveNManifests(t, total)

	out := captureStdout(t, func() {
		if err := printUndoList(false); err != nil {
			t.Fatalf("printUndoList(false): %v", err)
		}
	})

	lineCount := strings.Count(out, "/tmp/origem-")
	if lineCount != history.ListDisplayLimit {
		t.Fatalf("printUndoList(false) imprimiu %d linhas de manifesto, esperava %d (o limite padrão)", lineCount, history.ListDisplayLimit)
	}

	wantFooter := fmt.Sprintf("mostrando %d de %d — use --all para ver todos", history.ListDisplayLimit, total)
	if !strings.Contains(out, wantFooter) {
		t.Fatalf("saída não contém o rodapé esperado %q; saída:\n%s", wantFooter, out)
	}
}

// TestPrintUndoListAllShowsEverything prova que --all remove o limite
// padrão: todos os manifestos aparecem, e o rodapé de "mostrando N de M"
// não é impresso (não há nada escondido para avisar).
func TestPrintUndoListAllShowsEverything(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	total := history.ListDisplayLimit + 5
	saveNManifests(t, total)

	out := captureStdout(t, func() {
		if err := printUndoList(true); err != nil {
			t.Fatalf("printUndoList(true): %v", err)
		}
	})

	lineCount := strings.Count(out, "/tmp/origem-")
	if lineCount != total {
		t.Fatalf("printUndoList(true) imprimiu %d linhas de manifesto, esperava %d (todos)", lineCount, total)
	}

	if strings.Contains(out, "use --all para ver todos") {
		t.Fatalf("printUndoList(true) não deveria imprimir o rodapé de limite; saída:\n%s", out)
	}
}

// TestPrintUndoListWithinLimitShowsNoFooter garante que, com um histórico
// que já cabe no limite padrão, nenhum rodapé de "mostrando N de M" aparece
// — ele só faz sentido quando algo de fato ficou de fora.
func TestPrintUndoListWithinLimitShowsNoFooter(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	saveNManifests(t, 3)

	out := captureStdout(t, func() {
		if err := printUndoList(false); err != nil {
			t.Fatalf("printUndoList(false): %v", err)
		}
	})

	if strings.Contains(out, "use --all para ver todos") {
		t.Fatalf("printUndoList(false) não deveria imprimir rodapé quando tudo já coube; saída:\n%s", out)
	}
	if lineCount := strings.Count(out, "/tmp/origem-"); lineCount != 3 {
		t.Fatalf("printUndoList(false) imprimiu %d linhas, esperava 3", lineCount)
	}
}

// TestPrintUndoListWarnsAboutCorruptedManifest confirma que "undo --list"
// (via printUndoList) exibe o aviso de um manifesto ilegível — ao contrário
// da completação de --id, que deve ignorá-lo (ver
// TestUndoIDCompletionIgnoresCorruptedManifestWarnings, em
// completion_test.go): aqui é uma listagem pedida explicitamente pelo
// usuário, então o aviso tem lugar.
func TestPrintUndoListWarnsAboutCorruptedManifest(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	saveNManifests(t, 1)

	dir, err := history.Dir()
	if err != nil {
		t.Fatalf("history.Dir(): %v", err)
	}
	corrupted := dir + "/20261231-235959.yaml"
	if err := os.WriteFile(corrupted, []byte("isto: [nao é yaml válido"), 0o644); err != nil {
		t.Fatalf("criar manifesto corrompido: %v", err)
	}

	out := captureStdout(t, func() {
		if err := printUndoList(false); err != nil {
			t.Fatalf("printUndoList(false): %v", err)
		}
	})

	if !strings.Contains(out, "20261231-235959.yaml") {
		t.Fatalf("saída não menciona o manifesto corrompido; saída:\n%s", out)
	}
	if lineCount := strings.Count(out, "/tmp/origem-"); lineCount != 1 {
		t.Fatalf("printUndoList(false) deveria continuar mostrando o manifesto íntegro; linhas = %d, saída:\n%s", lineCount, out)
	}
}
