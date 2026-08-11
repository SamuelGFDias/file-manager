// Package undo implementa a tela interativa de desfazer uma operação:
// lista as operações registradas em internal/history, deixa o usuário
// escolher uma, mostra exatamente o que seria feito e pede confirmação
// antes de executar de verdade. É o equivalente, no menu principal, ao
// comando de linha de comando "file-manager undo" (internal/app/undo.go) —
// os dois chamam a mesma internal/history.Undo, então nunca podem divergir
// sobre o que "desfazer" significa.
package undo

import (
	"errors"
	"fmt"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"

	"github.com/SamuelGFDias/file-manager/internal/history"
	"github.com/SamuelGFDias/file-manager/internal/ui"
)

const optionBack = "Voltar"

// optionShowOlder reapresenta o seletor com o histórico completo, sem o
// limite de history.ListDisplayLimit — ver comentário de selectManifestID.
// Mesmo texto usado pelo equivalente em internal/app/undo.go (a linha de
// comando "undo" sem --id/--last): as duas telas nunca podem divergir sobre
// como lidar com um histórico grande, mesmo princípio já aplicado a
// history.Undo em todo o resto deste par de arquivos.
const optionShowOlder = "Ver operações mais antigas"

// screen é a implementação de ui.Screen para a tela de desfazer.
type screen struct{}

// NewScreen devolve a tela interativa de desfazer uma operação.
func NewScreen() ui.Screen {
	return &screen{}
}

// Title devolve o título exibido no breadcrumb.
func (s *screen) Title() string {
	return "Desfazer"
}

// Run lista as operações registradas, deixa o usuário escolher uma (ou
// voltar) e conduz o fluxo de confirmação/execução. Erros nunca derrubam o
// menu: qualquer falha é mostrada com ui.Errorf seguido de ui.Pause(), e a
// tela sempre retorna ao chamador (nav.Pop()) — nenhum caminho deste método
// devolve um erro não-nil.
func (s *screen) Run(nav *ui.Navigator) error {
	headers, warnings, err := history.List()
	if err != nil {
		ui.Errorf("erro ao listar operações registradas: %v", err)
		ui.Pause()
		nav.Pop()
		return nil
	}
	// Manifesto individual ilegível (ver history.List) não impede a tela
	// de abrir com o que sobrou — só avisa, do mesmo jeito que "undo
	// --list" na linha de comando.
	for _, w := range warnings {
		ui.Warnf("%s", w)
	}

	if len(headers) == 0 {
		// Não deveria acontecer na prática — mainmenu só empurra esta tela
		// quando já sabe que há histórico — mas a tela precisa se
		// comportar bem mesmo assim (ex: alguém apagou os manifestos à
		// mão entre a checagem do menu e a escolha).
		ui.Infof("Nenhuma operação registrada ainda.")
		ui.Pause()
		nav.Pop()
		return nil
	}

	id, ok, err := selectManifestID(headers)
	if err != nil {
		if !isInterrupt(err) {
			ui.Errorf("erro ao ler seleção: %v", err)
			ui.Pause()
		}
		nav.Pop()
		return nil
	}
	if !ok {
		nav.Pop()
		return nil
	}

	m, err := history.Load(id)
	if err != nil {
		ui.Errorf("erro ao carregar a operação selecionada: %v", err)
		ui.Pause()
		nav.Pop()
		return nil
	}

	runUndo(m)
	nav.Pop()
	return nil
}

// selectManifestID monta o seletor com no máximo history.ListDisplayLimit
// operações (headers já vem ordenado das mais recentes para as mais
// antigas — ver history.List), mais "Voltar". Com histórico maior que o
// limite, uma opção extra "Ver operações mais antigas" reapresenta o
// seletor com a lista completa, sem limite algum — um survey.Select com
// centenas de itens é inutilizável. ok é false quando o usuário escolheu
// "Voltar"; err carrega qualquer erro de leitura do prompt (ex: Ctrl+C).
func selectManifestID(headers []history.Header) (id string, ok bool, err error) {
	shown := headers
	truncated := len(headers) > history.ListDisplayLimit
	if truncated {
		shown = headers[:history.ListDisplayLimit]
	}

	for {
		options := make([]string, 0, len(shown)+2)
		byLabel := make(map[string]string, len(shown))
		for _, h := range shown {
			label := formatManifestLabel(h)
			options = append(options, label)
			byLabel[label] = h.ID
		}
		if truncated {
			options = append(options, optionShowOlder)
		}
		options = append(options, optionBack)

		chosen := ""
		if askErr := survey.AskOne(&survey.Select{
			Message: "Qual operação deseja desfazer?",
			Options: options,
		}, &chosen); askErr != nil {
			return "", false, askErr
		}

		if chosen == optionBack {
			return "", false, nil
		}
		if chosen == optionShowOlder {
			shown = headers
			truncated = false
			continue
		}

		return byLabel[chosen], true, nil
	}
}

// formatManifestLabel formata uma entrada do seletor: ID, data, ferramenta,
// ação, pastas, quantidade de arquivos e status (pendente ou já desfeita).
func formatManifestLabel(h history.Header) string {
	status := "pendente"
	if h.UndoneAt != nil {
		status = "já desfeita em " + h.UndoneAt.Local().Format("02/01/2006 15:04:05")
	}
	return fmt.Sprintf(
		"%s — %s — %s (%s) — %s → %s — %s — %s",
		h.ID,
		h.CreatedAt.Local().Format("02/01/2006 15:04:05"),
		h.Tool,
		h.Action,
		h.InputDir,
		h.OutputDir,
		ui.Count(h.EntryCount, "arquivo", "arquivos"),
		status,
	)
}

// runUndo calcula o plano (sempre — é a base tanto da mensagem informativa
// quanto da confirmação), pede confirmação e, se aceita, executa de
// verdade. Nunca devolve erro: qualquer falha é reportada com ui.Errorf +
// ui.Pause() e a função apenas retorna.
func runUndo(m history.Manifest) {
	plan, err := history.Undo(m, true, false)
	if err != nil {
		if errors.Is(err, history.ErrAlreadyUndone) {
			ui.Warnf(
				"esta operação já foi desfeita em %s. Use a linha de comando (\"file-manager undo --id %s --force\") se realmente deseja tentar de novo.",
				m.UndoneAt.Local().Format("02/01/2006 15:04:05"), m.ID,
			)
			ui.Pause()
			return
		}
		ui.Errorf("erro ao calcular o que seria desfeito: %v", err)
		ui.Pause()
		return
	}

	// A tela não tem um equivalente a --dry-run pedido explicitamente,
	// então NUNCA passa previewRequested=true a BuildUndoReport — a
	// palavra "simulação" não tem lugar aqui. Quando não há nada a
	// restaurar, o relatório (que distingue "nada a fazer" de "preservado
	// por segurança") já é a última coisa a mostrar; não há confirmação a
	// pedir nem execução real a fazer.
	if len(plan.Restored) == 0 {
		printUndoReport(history.BuildUndoReport(plan, false, nil), false)
		ui.Pause()
		return
	}

	confirmed := false
	promptErr := survey.AskOne(&survey.Confirm{
		Message: fmt.Sprintf("Confirma desfazer, afetando %s?", ui.Count(len(plan.Restored), "arquivo", "arquivos")),
		Default: false,
	}, &confirmed)
	if promptErr != nil {
		if !isInterrupt(promptErr) {
			ui.Errorf("erro ao ler confirmação: %v", promptErr)
			ui.Pause()
		}
		return
	}
	if !confirmed {
		return
	}

	result, err := history.Undo(m, false, false)
	if err != nil {
		ui.Errorf("erro ao desfazer: %v", err)
		ui.Pause()
		return
	}

	if err := history.MarkUndone(m.ID, time.Now()); err != nil {
		ui.Warnf("aviso: desfazer concluído, mas não foi possível marcar a operação como desfeita no histórico: %v", err)
	}

	// Único ponto de impressão do resultado real: plan nunca é impresso
	// separadamente em nenhum outro lugar deste fluxo, o que impede o
	// resumo de aparecer duas vezes.
	printUndoReport(history.BuildUndoReport(plan, false, &result), true)
	ui.Pause()
}

// printUndoReport imprime um history.UndoReport na ordem definida por
// Lines(): motivos de arquivos pulados primeiro (sempre um aviso), resumo
// final por último. succeeded controla só o estilo do resumo final:
// ui.Successf (✓) quando algo foi de fato restaurado, ui.Infof para
// qualquer outro caso — nunca um "✓" para algo que não aconteceu de
// verdade.
func printUndoReport(r history.UndoReport, succeeded bool) {
	for _, line := range r.Skipped {
		ui.Warnf("%s", line)
	}
	if succeeded {
		ui.Successf("%s", r.Summary)
		return
	}
	ui.Infof("%s", r.Summary)
}

// isInterrupt indica se err é (ou envolve) terminal.InterruptErr, sinal de
// que o usuário pressionou Ctrl+C.
func isInterrupt(err error) bool {
	return errors.Is(err, terminal.InterruptErr)
}
