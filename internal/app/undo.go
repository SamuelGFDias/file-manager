package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"

	"github.com/SamuelGFDias/file-manager/internal/history"
	"github.com/SamuelGFDias/file-manager/internal/ui"
)

// optionShowOlder é a opção extra oferecida no seletor interativo de "undo"
// (sem --id/--last, em terminal interativo) quando o histórico tem mais de
// history.ListDisplayLimit operações — escolhê-la reapresenta o seletor com
// a lista completa. Um survey.Select com centenas de itens é inutilizável;
// ver o mesmo padrão em internal/ui/undo/screen.go (a tela equivalente do
// menu principal), que nunca pode divergir deste comando sobre como lidar
// com um histórico grande.
const optionShowOlder = "Ver operações mais antigas"

// newUndoCommand monta o subcomando "undo": desfaz uma operação registrada
// por uma ferramenta que suporta desfazer (hoje, só organize-pdf). A
// reversão em si (verificação de tamanho, "não sobrescreva a origem",
// remoção de diretórios vazios) fica inteiramente em internal/history —
// este comando só resolve QUAL manifesto usar, mostra o plano e pede
// confirmação.
func newUndoCommand() *cobra.Command {
	var id string
	var last bool
	var dryRun bool
	var yes bool
	var list bool
	var all bool
	var force bool
	var prune bool
	var olderThan int

	cmd := &cobra.Command{
		Use:   "undo",
		Short: "Desfaz uma operação registrada (hoje, só organize-pdf)",
		Long: "Desfaz uma operação registrada por uma ferramenta que suporta desfazer: para uma " +
			"operação de cópia, apaga os arquivos criados no destino (o original nunca é tocado); " +
			"para uma operação de mover, devolve os arquivos à pasta de origem. Nunca toca em um " +
			"arquivo fora do que foi registrado na hora da operação original; pula (em vez de " +
			"apagar ou sobrescrever) qualquer arquivo cujo tamanho tenha mudado desde então, ou cuja " +
			"origem já esteja ocupada por outro arquivo. Só funciona para operações reais feitas a " +
			"partir da versão que introduziu esse recurso — uma simulação (--dry-run em organize-pdf) " +
			"nunca gera histórico. O histórico é mantido por um tempo limitado (ver --prune) e a " +
			"listagem (--list) mostra por padrão só as operações mais recentes (ver --all).",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if prune {
				return runUndoPrune(olderThan, yes)
			}

			if list {
				return printUndoList(all)
			}

			m, err := resolveUndoManifest(id, last)
			if err != nil {
				return err
			}

			return runUndoCommand(m, dryRun, yes, force)
		},
	}

	cmd.Flags().StringVar(&id, "id", "", "ID da operação a desfazer (ver \"file-manager undo --list\")")
	cmd.Flags().BoolVar(&last, "last", false, "Desfaz a operação registrada mais recente")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Só mostra o que seria feito, sem tocar em nada")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Desfaz sem pedir confirmação")
	cmd.Flags().BoolVar(&list, "list", false, "Lista as operações registradas e sai")
	cmd.Flags().BoolVar(&all, "all", false, "Com --list, mostra todas as operações, sem o limite padrão")
	cmd.Flags().BoolVar(&force, "force", false, "Permite desfazer uma operação que já foi desfeita antes")
	cmd.Flags().BoolVar(&prune, "prune", false, "Remove do disco os manifestos de histórico expirados e sai")
	cmd.Flags().IntVar(&olderThan, "older-than", 0,
		"Com --prune, usa N dias como limiar em vez do padrão (30 dias para já desfeitas, 180 para pendentes)")
	_ = cmd.RegisterFlagCompletionFunc("id", undoIDCompletion)

	return cmd
}

// undoIDCompletion completa "--id" com os manifestos ainda não desfeitos,
// na mesma ordem de history.List() (mais recentes primeiro), no formato
// "<ID>\t<data> — <pasta de origem>" — o texto depois do TAB vira a
// descrição mostrada pelo zsh ao lado de cada opção. Só oferece manifestos
// com UndoneAt vazio: sugerir um ID já desfeito levaria o usuário direto a
// um erro evitável ("já foi desfeita em ..."). Qualquer erro ao listar o
// histórico (ex: diretório de configuração inacessível) devolve lista
// vazia sem propagar o erro — completação nunca pode falhar ruidosamente.
// Warnings de manifestos individuais ilegíveis (ver history.List) também
// são ignorados aqui, de propósito: um Tab não pode cuspir aviso no meio da
// linha de comando que o usuário está digitando.
func undoIDCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	headers, _, err := history.List()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	out := make([]string, 0, len(headers))
	for _, h := range headers {
		if h.UndoneAt != nil {
			continue
		}
		out = append(out, fmt.Sprintf(
			"%s\t%s — %s",
			h.ID, h.CreatedAt.Local().Format("02/01/2006 15:04:05"), h.InputDir,
		))
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// printUndoList lista as operações registradas (ID, data, ferramenta,
// pastas, ação, quantidade de arquivos, e se já foi desfeita). Mostra, por
// padrão, só as history.ListDisplayLimit mais recentes, com um rodapé
// avisando quantas ficaram de fora; all mostra todas. Qualquer warning
// devolvido por history.List (manifesto individual ilegível) é impresso
// antes da lista — ao contrário da completação de --id, aqui é uma
// listagem pedida explicitamente pelo usuário, então o aviso tem lugar.
func printUndoList(all bool) error {
	headers, warnings, err := history.List()
	if err != nil {
		return fmt.Errorf("erro ao listar operações registradas: %w", err)
	}

	for _, w := range warnings {
		ui.Warnf("%s", w)
	}

	if len(headers) == 0 {
		ui.Infof("Nenhuma operação registrada ainda.")
		return nil
	}

	shown := headers
	if !all && len(headers) > history.ListDisplayLimit {
		shown = headers[:history.ListDisplayLimit]
	}

	for _, h := range shown {
		ui.Infof("%s", undoListLine(h))
	}

	if len(shown) < len(headers) {
		ui.Infof("mostrando %d de %d — use --all para ver todos", len(shown), len(headers))
	}

	return nil
}

// undoListLine formata uma linha de "file-manager undo --list" e também a
// opção correspondente do seletor interativo.
func undoListLine(h history.Header) string {
	status := "pendente"
	if h.UndoneAt != nil {
		status = "desfeita em " + h.UndoneAt.Local().Format("02/01/2006 15:04:05")
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

// resolveUndoManifest decide qual manifesto desfazer: --id tem prioridade
// absoluta, depois --last (o mais recente), e por fim — só em terminal
// interativo — uma pergunta com survey.Select. Sem terminal interativo e
// sem --id/--last, devolve um erro claro em vez de travar esperando input
// que nunca vai chegar.
func resolveUndoManifest(id string, last bool) (history.Manifest, error) {
	if id != "" {
		return history.Load(id)
	}

	headers, warnings, err := history.List()
	if err != nil {
		return history.Manifest{}, fmt.Errorf("erro ao listar operações registradas: %w", err)
	}
	for _, w := range warnings {
		ui.Warnf("%s", w)
	}
	if len(headers) == 0 {
		return history.Manifest{}, fmt.Errorf("nenhuma operação registrada ainda; não há o que desfazer")
	}

	if last {
		return history.Load(headers[0].ID)
	}

	if !ui.IsInteractive() {
		return history.Manifest{}, fmt.Errorf(
			"nenhum terminal interativo disponível; informe --id ou --last para escolher a operação a desfazer " +
				"(veja as opções com \"file-manager undo --list\")",
		)
	}

	id, err = selectManifestID(headers)
	if err != nil {
		return history.Manifest{}, err
	}
	return history.Load(id)
}

// selectManifestID monta o seletor interativo de qual operação desfazer,
// limitado às history.ListDisplayLimit mais recentes (headers já vem
// ordenado assim), com uma opção extra "Ver operações mais antigas" quando
// o histórico é maior — escolhê-la reapresenta o seletor com a lista
// completa, sem limite algum. Mesmo padrão usado pela tela interativa
// equivalente (internal/ui/undo/screen.go), para que as duas nunca possam
// divergir sobre como lidar com um histórico grande.
func selectManifestID(headers []history.Header) (string, error) {
	shown := headers
	truncated := len(headers) > history.ListDisplayLimit
	if truncated {
		shown = headers[:history.ListDisplayLimit]
	}

	for {
		options := make([]string, 0, len(shown)+1)
		byLabel := make(map[string]string, len(shown))
		for _, h := range shown {
			label := undoListLine(h)
			options = append(options, label)
			byLabel[label] = h.ID
		}
		if truncated {
			options = append(options, optionShowOlder)
		}

		chosen := ""
		if err := survey.AskOne(&survey.Select{
			Message: "Qual operação deseja desfazer?",
			Options: options,
		}, &chosen); err != nil {
			return "", err
		}

		if chosen == optionShowOlder {
			shown = headers
			truncated = false
			continue
		}

		return byLabel[chosen], nil
	}
}

// runUndoCommand conduz o restante do fluxo depois que o manifesto já foi
// escolhido: mostra o plano (sempre — mesmo fora de --dry-run, é a base da
// confirmação), pede confirmação a menos que --yes tenha sido informado, e
// executa de verdade.
func runUndoCommand(m history.Manifest, dryRun, yes, force bool) error {
	plan, err := history.Undo(m, true, force)
	if err != nil {
		if errors.Is(err, history.ErrAlreadyUndone) {
			return fmt.Errorf(
				"a operação %q já foi desfeita em %s; use --force se realmente deseja tentar de novo",
				m.ID, m.UndoneAt.Local().Format("02/01/2006 15:04:05"),
			)
		}
		return err
	}

	// dryRun (a flag --dry-run) é o único caso em que o usuário pediu uma
	// prévia de propósito — é o único caso em que o rótulo "[simulação]"
	// pode aparecer. BuildUndoReport(plan, dryRun, nil) garante isso: o
	// segundo argumento nunca é derivado de plan.DryRun (que é sempre
	// true internamente, pedido --dry-run ou não).
	if dryRun {
		printUndoReport(history.BuildUndoReport(plan, true, nil), false)
		return nil
	}

	// Nada seria restaurado mesmo executando de verdade: nenhuma
	// confirmação a pedir, nenhuma execução real a fazer. O relatório usa
	// plan.Outcome() (via BuildUndoReport com previewRequested=false e
	// final=nil), que distingue "nada a fazer" de "preservado por
	// segurança" — nunca a palavra "simulação", porque esta não foi uma
	// prévia pedida pelo usuário.
	if len(plan.Restored) == 0 {
		printUndoReport(history.BuildUndoReport(plan, false, nil), false)
		return nil
	}

	if !yes {
		confirmed := false
		if err := survey.AskOne(&survey.Confirm{
			Message: fmt.Sprintf("Confirma desfazer, afetando %s?", ui.Count(len(plan.Restored), "arquivo", "arquivos")),
			Default: false,
		}, &confirmed); err != nil {
			return err
		}
		if !confirmed {
			ui.Infof("desfazer cancelado")
			return nil
		}
	}

	result, err := history.Undo(m, false, force)
	if err != nil {
		return err
	}

	if err := history.MarkUndone(m.ID, time.Now()); err != nil {
		ui.Warnf("aviso: desfazer concluído, mas não foi possível marcar a operação como desfeita no histórico: %v", err)
	}

	// Único ponto de impressão do resultado real: nem plan nem result são
	// impressos separadamente em nenhum outro lugar deste fluxo, o que
	// impede o resumo de aparecer duas vezes.
	printUndoReport(history.BuildUndoReport(plan, false, &result), true)

	return nil
}

// printUndoReport imprime um history.UndoReport na ordem definida por
// Lines(): motivos de arquivos pulados primeiro (sempre um aviso — cada
// linha já explica, por si, uma decisão de segurança), resumo final por
// último. succeeded controla só o estilo do resumo final: ui.Successf
// (✓) quando algo foi de fato restaurado numa execução real, ui.Infof
// para qualquer outro caso (prévia, ou execução real que preservou tudo
// por segurança) — nunca um "✓" para algo que não aconteceu de verdade.
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

// runUndoPrune executa "undo --prune": calcula o que seria removido (via
// history.PrunePlan, o mesmo padrão dryRun-depois-execução usado em todo o
// resto deste comando), mostra um resumo e pede confirmação — a menos que
// --yes tenha sido informado —, e só então remove de verdade
// (history.Prune). olderThanDays, quando > 0, substitui os dois limiares
// padrão (history.PruneUndoneAfter/PrunePendingAfter) pelo mesmo número de
// dias para os dois: não há hoje um caso de uso que peça limiares
// diferentes para pendentes e já desfeitas numa poda MANUAL, então um único
// número mantém a flag simples.
func runUndoPrune(olderThanDays int, yes bool) error {
	undoneAfter := history.PruneUndoneAfter
	pendingAfter := history.PrunePendingAfter
	if olderThanDays > 0 {
		d := time.Duration(olderThanDays) * 24 * time.Hour
		undoneAfter, pendingAfter = d, d
	}

	now := time.Now()
	pending, undone, err := history.PrunePlan(now, undoneAfter, pendingAfter)
	if err != nil {
		return fmt.Errorf("erro ao calcular a poda do histórico: %w", err)
	}

	total := len(pending) + len(undone)
	if total == 0 {
		ui.Infof("Nenhum manifesto de histórico elegível para poda.")
		return nil
	}

	ui.Infof(
		"%s elegíveis para remoção: %s já desfeitas, %s pendentes (perdem a capacidade de ser desfeitas).",
		ui.Count(total, "manifesto", "manifestos"),
		ui.Count(len(undone), "já desfeita", "já desfeitas"),
		ui.Count(len(pending), "pendente", "pendentes"),
	)

	if !yes {
		confirmed := false
		if err := survey.AskOne(&survey.Confirm{
			Message: fmt.Sprintf("Confirma remover %s do histórico?", ui.Count(total, "manifesto", "manifestos")),
			Default: false,
		}, &confirmed); err != nil {
			return err
		}
		if !confirmed {
			ui.Infof("poda cancelada")
			return nil
		}
	}

	removed, err := history.Prune(now, undoneAfter, pendingAfter)
	if err != nil {
		return fmt.Errorf("erro ao podar o histórico: %w", err)
	}

	ui.Successf("%s removidos do histórico.", ui.Count(len(removed), "manifesto", "manifestos"))
	return nil
}
