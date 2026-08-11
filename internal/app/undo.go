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
	var force bool

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
			"nunca gera histórico.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if list {
				return printUndoList()
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
	cmd.Flags().BoolVar(&force, "force", false, "Permite desfazer uma operação que já foi desfeita antes")

	return cmd
}

// printUndoList lista as operações registradas (ID, data, ferramenta,
// pastas, ação, quantidade de arquivos, e se já foi desfeita).
func printUndoList() error {
	manifests, err := history.List()
	if err != nil {
		return fmt.Errorf("erro ao listar operações registradas: %w", err)
	}

	if len(manifests) == 0 {
		ui.Infof("Nenhuma operação registrada ainda.")
		return nil
	}

	for _, m := range manifests {
		ui.Infof("%s", undoListLine(m))
	}

	return nil
}

// undoListLine formata uma linha de "file-manager undo --list" e também a
// opção correspondente do seletor interativo.
func undoListLine(m history.Manifest) string {
	status := "pendente"
	if m.UndoneAt != nil {
		status = "desfeita em " + m.UndoneAt.Local().Format("02/01/2006 15:04:05")
	}
	return fmt.Sprintf(
		"%s — %s — %s (%s) — %s → %s — %s — %s",
		m.ID,
		m.CreatedAt.Local().Format("02/01/2006 15:04:05"),
		m.Tool,
		m.Action,
		m.InputDir,
		m.OutputDir,
		ui.Count(len(m.Entries), "arquivo", "arquivos"),
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

	manifests, err := history.List()
	if err != nil {
		return history.Manifest{}, fmt.Errorf("erro ao listar operações registradas: %w", err)
	}
	if len(manifests) == 0 {
		return history.Manifest{}, fmt.Errorf("nenhuma operação registrada ainda; não há o que desfazer")
	}

	if last {
		return manifests[0], nil
	}

	if !ui.IsInteractive() {
		return history.Manifest{}, fmt.Errorf(
			"nenhum terminal interativo disponível; informe --id ou --last para escolher a operação a desfazer " +
				"(veja as opções com \"file-manager undo --list\")",
		)
	}

	options := make([]string, 0, len(manifests))
	byLabel := make(map[string]history.Manifest, len(manifests))
	for _, m := range manifests {
		label := undoListLine(m)
		options = append(options, label)
		byLabel[label] = m
	}

	chosen := ""
	if err := survey.AskOne(&survey.Select{
		Message: "Qual operação deseja desfazer?",
		Options: options,
	}, &chosen); err != nil {
		return history.Manifest{}, err
	}

	return byLabel[chosen], nil
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
