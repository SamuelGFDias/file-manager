// Package mainmenu implementa a tela inicial do modo interativo do CLI
// file-manager: o menu que lista as ferramentas registradas e dá acesso às
// telas de perfis e de documentação.
package mainmenu

import (
	"errors"
	"fmt"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"

	"github.com/SamuelGFDias/file-manager/internal/commanddocs"
	"github.com/SamuelGFDias/file-manager/internal/history"
	"github.com/SamuelGFDias/file-manager/internal/selfupdate"
	"github.com/SamuelGFDias/file-manager/internal/tool"
	"github.com/SamuelGFDias/file-manager/internal/ui"
	"github.com/SamuelGFDias/file-manager/internal/ui/docs"
	"github.com/SamuelGFDias/file-manager/internal/ui/profiles"
	"github.com/SamuelGFDias/file-manager/internal/ui/undo"
)

const (
	optionProfiles = "Perfis"
	optionUndo     = "Desfazer uma organização"
	optionDocs     = "Documentação"
	optionExit     = "Sair"
)

// updateNoticeWaitTimeout é o tempo máximo que a primeira renderização do
// menu espera pelo resultado da checagem de atualização antes de desistir e
// abrir o menu sem aviso. A chamada à API do GitHub leva ~250ms em condições
// normais medidas em produção; 1,5s dá folga larga sem ser perceptível para
// quem abre a ferramenta, e ainda garante um teto curto quando não há rede.
const updateNoticeWaitTimeout = 1500 * time.Millisecond

// screen é a implementação de ui.Screen para o menu principal.
type screen struct {
	tools   []tool.Tool
	version string

	updateChecker *selfupdate.Checker

	// firstRender é true até a primeira renderização do menu terminar de
	// resolver o aviso de atualização; depois disso, false. Controla se
	// Run() paga a espera limitada de WaitNotice (só vale a pena uma vez,
	// para dar tempo da checagem em segundo plano terminar antes do menu
	// aparecer) ou usa Notice(), não bloqueante, nas vezes seguintes — o
	// resultado já estará pronto havendo passado por WaitNotice antes.
	firstRender bool
}

// NewScreen devolve a tela do menu principal, listando as ferramentas
// informadas (mais "Perfis", quando alguma delas suportar perfis salvos,
// "Documentação" e "Sair").
//
// É também o ponto de entrada de toda sessão interativa, então é aqui que
// ui.ApplyTheme() é chamada — uma única vez, de forma idempotente — para
// que o template de seleção do survey (descrição só na opção selecionada,
// dica em português) já esteja em vigor antes do primeiro prompt.
//
// version é a versão formatada exibida no cabeçalho do menu (ex: "0.1.0
// (abc1234, 2026-08-11T12:00:00Z)"); currentVersion é a versão semântica
// crua (ex: "v0.1.0", ou "dev" em builds locais), usada para checar em
// segundo plano se há uma versão mais nova publicada. A checagem é
// disparada uma única vez aqui (Start é idempotente e não bloqueia) para
// que, se houver aviso, ele já esteja pronto quando o rodapé do menu for
// desenhado, sem nunca atrasar a abertura do menu nem repetir a consulta a
// cada redesenho da tela.
func NewScreen(tools []tool.Tool, version string, currentVersion string) ui.Screen {
	ui.ApplyTheme()

	checker := selfupdate.NewChecker(selfupdate.DefaultRepo, currentVersion)
	checker.Start()

	return &screen{tools: tools, version: version, updateChecker: checker, firstRender: true}
}

// Title devolve o título da tela, usado no breadcrumb de navegação.
func (s *screen) Title() string {
	return "File Manager"
}

// Run mostra o menu principal e navega para a tela correspondente à opção
// escolhida.
func (s *screen) Run(nav *ui.Navigator) error {
	options := make([]string, 0, len(s.tools)+3)
	descriptions := make(map[string]string, len(s.tools))
	toolByLabel := make(map[string]tool.Tool, len(s.tools))

	for _, t := range s.tools {
		meta := t.Meta()
		options = append(options, meta.Title)
		descriptions[meta.Title] = meta.Description
		toolByLabel[meta.Title] = t
	}

	if len(profiles.SupportingTools(s.tools)) > 0 {
		options = append(options, optionProfiles)
	}
	// "Desfazer uma organização" só aparece quando já existe pelo menos um
	// manifesto registrado — não faz sentido oferecer desfazer a quem
	// nunca organizou nada, e a lista ficaria poluída de opção vazia.
	// Erro ao listar é tratado como "sem histórico" aqui (silenciosamente,
	// sem travar a abertura do menu): o próprio comando "undo" e a tela
	// undo.NewScreen() reportam o erro de verdade se o usuário de fato
	// tentar entrar nela por outro caminho; a checagem aqui é só para
	// decidir se a opção aparece.
	if headers, _, err := history.List(); err == nil && len(headers) > 0 {
		options = append(options, optionUndo)
	}
	options = append(options, optionDocs, optionExit)

	// Aviso de atualização, no rodapé do menu: impresso antes de abrir o
	// survey.Select porque, uma vez aberto, o select toma conta do terminal
	// e qualquer impressão feita depois some por trás dele.
	//
	// Na primeira renderização, o resultado da checagem em segundo plano
	// (disparada em NewScreen) normalmente ainda não chegou — a chamada à
	// API do GitHub leva ~250ms — então usar Notice() aqui faria o aviso
	// nunca aparecer na prática, já que o survey.Select assume o terminal
	// logo em seguida e o menu não é redesenhado enquanto o usuário navega.
	// WaitNotice dá à checagem uma folga curta e limitada para terminar
	// antes de abrir o seletor, sem nunca travar a abertura do menu além do
	// teto do timeout (e sem esperar nada quando a versão local não é
	// semver, ou quando a checagem já tiver terminado). Nas renderizações
	// seguintes (o usuário voltou de uma tela), o resultado já está pronto e
	// Notice() devolve-o instantaneamente, sem pagar a espera de novo.
	var notice string
	var ok bool
	if s.firstRender {
		notice, ok = s.updateChecker.WaitNotice(updateNoticeWaitTimeout)
		s.firstRender = false
	} else {
		notice, ok = s.updateChecker.Notice()
	}
	if ok {
		// Correção de defeito e mudança incompatível recebem o mesmo
		// destaque visual (ui.Warnf); novidade pura (SeverityMinor) não
		// precisa da mesma urgência — ui.Infof basta.
		switch s.updateChecker.Severity() {
		case selfupdate.SeverityPatch, selfupdate.SeverityMajor:
			ui.Warnf("%s", notice)
		default:
			ui.Infof("%s", notice)
		}
	}

	choice := ""
	err := survey.AskOne(&survey.Select{
		Message: fmt.Sprintf("File Manager (%s) — o que você deseja fazer?", s.version),
		Options: options,
		Description: func(value string, index int) string {
			return descriptions[value]
		},
	}, &choice)
	if err != nil {
		if errors.Is(err, terminal.InterruptErr) {
			nav.Exit()
			return nil
		}
		return err
	}

	switch choice {
	case optionProfiles:
		nav.Push(profiles.NewScreen(s.tools))
	case optionUndo:
		nav.Push(undo.NewScreen())
	case optionDocs:
		nav.Push(docs.NewScreen(s.tools, commanddocs.CommandDocs(), s.version))
	case optionExit:
		nav.Exit()
	default:
		if t, ok := toolByLabel[choice]; ok {
			nav.Push(t.Screen())
		}
	}

	return nil
}
