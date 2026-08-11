// Package profiles implementa a tela genérica de gestão de perfis do modo
// interativo. É uma única tela que serve todas as ferramentas do CLI que
// suportam perfis (tool.ProfileSupport != nil): nenhuma ferramenta
// reimplementa o CRUD de perfil por conta própria.
package profiles

import (
	"errors"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"

	"github.com/SamuelGFDias/file-manager/internal/config"
	"github.com/SamuelGFDias/file-manager/internal/tool"
	"github.com/SamuelGFDias/file-manager/internal/ui"
)

const (
	actionList   = "Listar"
	actionCreate = "Criar"
	actionEdit   = "Editar"
	actionDup    = "Duplicar"
	actionDelete = "Excluir"
	actionApply  = "Aplicar agora"
	actionBack   = "Voltar"
	maxNameTries = 3
)

// screen é a implementação de ui.Screen para a tela de gestão de perfis.
type screen struct {
	tools []tool.Tool
}

// NewScreen devolve a tela de gestão de perfis para as ferramentas
// informadas. Ferramentas cujo Profile() é nil são ignoradas
// automaticamente.
func NewScreen(tools []tool.Tool) ui.Screen {
	return &screen{tools: tools}
}

// SupportingTools filtra a lista, devolvendo só as ferramentas com
// Profile() != nil. Exportada porque também é usada para decidir se a
// entrada "Perfis" aparece no menu principal.
func SupportingTools(tools []tool.Tool) []tool.Tool {
	out := make([]tool.Tool, 0, len(tools))
	for _, t := range tools {
		if t.Profile() != nil {
			out = append(out, t)
		}
	}
	return out
}

// Title retorna o nome de exibição da tela.
func (s *screen) Title() string {
	return "Perfis"
}

// Run executa o laço da tela: escolhe a ferramenta e, em seguida, laça sobre
// o menu de ações até o usuário voltar.
func (s *screen) Run(nav *ui.Navigator) error {
	supported := SupportingTools(s.tools)
	if len(supported) == 0 {
		ui.Warnf("Nenhuma ferramenta deste CLI suporta perfis salvos.")
		ui.Pause()
		nav.Pop()
		return nil
	}

	toolOptions := make([]string, 0, len(supported)+1)
	toolByLabel := make(map[string]tool.Tool, len(supported))
	for _, t := range supported {
		label := t.Meta().Title
		toolOptions = append(toolOptions, label)
		toolByLabel[label] = t
	}
	toolOptions = append(toolOptions, actionBack)

	chosenLabel := ""
	err := survey.AskOne(&survey.Select{
		Message: "Escolha a ferramenta:",
		Options: toolOptions,
	}, &chosenLabel)
	if err != nil {
		if isInterrupt(err) {
			nav.Pop()
			return nil
		}
		return err
	}

	if chosenLabel == actionBack {
		nav.Pop()
		return nil
	}

	t, ok := toolByLabel[chosenLabel]
	if !ok {
		nav.Pop()
		return nil
	}

	s.runActionsMenu(nav, t)
	return nil
}

// runActionsMenu laça sobre o menu de ações para a ferramenta escolhida até
// que o usuário selecione "Voltar" (ou interrompa com Ctrl+C).
func (s *screen) runActionsMenu(nav *ui.Navigator, t tool.Tool) {
	actionOptions := []string{
		actionList,
		actionCreate,
		actionEdit,
		actionDup,
		actionDelete,
		actionApply,
		actionBack,
	}

	for {
		action := ""
		err := survey.AskOne(&survey.Select{
			Message: "Perfis de " + t.Meta().Title + " — escolha uma ação:",
			Options: actionOptions,
		}, &action)
		if err != nil {
			if isInterrupt(err) {
				return
			}
			// Erro inesperado do survey que não seja interrupção: trata
			// como falha de I/O comum, avisa e continua o laço de ações.
			ui.Errorf("erro ao ler seleção: %v", err)
			ui.Pause()
			continue
		}

		switch action {
		case actionBack:
			return
		case actionList:
			s.doList(t)
		case actionCreate:
			s.doCreate(nav, t)
		case actionEdit:
			s.doEdit(nav, t)
		case actionDup:
			s.doDuplicate(t)
		case actionDelete:
			s.doDelete(t)
		case actionApply:
			s.doApply(t)
		}
	}
}

// doList lista os perfis existentes da ferramenta.
func (s *screen) doList(t tool.Tool) {
	toolID := t.Meta().ID
	names, err := config.List(toolID)
	if err != nil {
		ui.Errorf("erro ao listar perfis: %v", err)
		ui.Pause()
		return
	}

	if len(names) == 0 {
		ui.Infof("Ainda não há perfis salvos para %s.", t.Meta().Title)
		ui.Pause()
		return
	}

	for _, name := range names {
		path, err := config.ProfilePath(toolID, name)
		if err != nil {
			ui.Errorf("erro ao resolver caminho do perfil %q: %v", name, err)
			continue
		}
		ui.Infof("%s — %s", name, path)
	}
	ui.Pause()
}

// doCreate cria um novo perfil, perguntando o nome e coletando as opções via
// Profile().Edit.
func (s *screen) doCreate(nav *ui.Navigator, t tool.Tool) {
	toolID := t.Meta().ID

	name, ok := askNewProfileName()
	if !ok {
		return
	}

	exists, err := config.Exists(toolID, name)
	if err != nil {
		ui.Errorf("erro ao verificar perfil existente: %v", err)
		ui.Pause()
		return
	}

	if exists {
		overwrite := false
		promptErr := survey.AskOne(&survey.Confirm{
			Message: "Já existe um perfil chamado \"" + name + "\". Sobrescrever?",
			Default: false,
		}, &overwrite)
		if promptErr != nil {
			if !isInterrupt(promptErr) {
				ui.Errorf("erro ao ler confirmação: %v", promptErr)
				ui.Pause()
			}
			return
		}
		if !overwrite {
			return
		}
	}

	opts, err := t.Profile().Edit(nav, t.Profile().Empty())
	if err != nil {
		if !isInterrupt(err) {
			ui.Errorf("erro ao coletar opções do perfil: %v", err)
			ui.Pause()
		}
		return
	}

	if err := config.Save(toolID, name, opts); err != nil {
		ui.Errorf("erro ao salvar perfil: %v", err)
		ui.Pause()
		return
	}

	ui.Successf("Perfil %q salvo com sucesso.", name)
}

// doEdit edita um perfil existente, reaproveitando as perguntas de
// Profile().Edit.
func (s *screen) doEdit(nav *ui.Navigator, t tool.Tool) {
	toolID := t.Meta().ID

	name, ok := s.pickExistingProfile(t, "Escolha o perfil para editar:")
	if !ok {
		return
	}

	current := t.Profile().Empty()
	if err := config.Load(toolID, name, current); err != nil {
		ui.Errorf("erro ao carregar perfil %q: %v", name, err)
		ui.Pause()
		return
	}

	edited, err := t.Profile().Edit(nav, current)
	if err != nil {
		if !isInterrupt(err) {
			ui.Errorf("erro ao editar perfil: %v", err)
			ui.Pause()
		}
		return
	}

	if err := config.Save(toolID, name, edited); err != nil {
		ui.Errorf("erro ao salvar perfil: %v", err)
		ui.Pause()
		return
	}

	ui.Successf("Perfil %q atualizado com sucesso.", name)
}

// doDuplicate duplica um perfil existente sob um novo nome, sem passar pelas
// perguntas de Edit.
func (s *screen) doDuplicate(t tool.Tool) {
	toolID := t.Meta().ID

	src, ok := s.pickExistingProfile(t, "Escolha o perfil de origem:")
	if !ok {
		return
	}

	newName, ok := askNewProfileName()
	if !ok {
		return
	}

	exists, err := config.Exists(toolID, newName)
	if err != nil {
		ui.Errorf("erro ao verificar perfil existente: %v", err)
		ui.Pause()
		return
	}
	if exists {
		overwrite := false
		promptErr := survey.AskOne(&survey.Confirm{
			Message: "Já existe um perfil chamado \"" + newName + "\". Sobrescrever?",
			Default: false,
		}, &overwrite)
		if promptErr != nil {
			if !isInterrupt(promptErr) {
				ui.Errorf("erro ao ler confirmação: %v", promptErr)
				ui.Pause()
			}
			return
		}
		if !overwrite {
			return
		}
	}

	current := t.Profile().Empty()
	if err := config.Load(toolID, src, current); err != nil {
		ui.Errorf("erro ao carregar perfil %q: %v", src, err)
		ui.Pause()
		return
	}

	if err := config.Save(toolID, newName, current); err != nil {
		ui.Errorf("erro ao salvar perfil: %v", err)
		ui.Pause()
		return
	}

	ui.Successf("Perfil %q duplicado para %q com sucesso.", src, newName)
}

// doDelete exclui um perfil existente, mediante confirmação.
func (s *screen) doDelete(t tool.Tool) {
	toolID := t.Meta().ID

	name, ok := s.pickExistingProfile(t, "Escolha o perfil para excluir:")
	if !ok {
		return
	}

	confirmed := false
	promptErr := survey.AskOne(&survey.Confirm{
		Message: "Tem certeza que deseja excluir o perfil \"" + name + "\"?",
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

	if err := config.Delete(toolID, name); err != nil {
		ui.Errorf("erro ao excluir perfil: %v", err)
		ui.Pause()
		return
	}

	ui.Successf("Perfil %q excluído com sucesso.", name)
}

// doApply carrega um perfil existente e executa a ferramenta com ele.
func (s *screen) doApply(t tool.Tool) {
	toolID := t.Meta().ID

	name, ok := s.pickExistingProfile(t, "Escolha o perfil para aplicar:")
	if !ok {
		return
	}

	opts := t.Profile().Empty()
	if err := config.Load(toolID, name, opts); err != nil {
		ui.Errorf("erro ao carregar perfil %q: %v", name, err)
		ui.Pause()
		return
	}

	result, err := t.Profile().Apply(opts)
	if err != nil {
		ui.Errorf("erro ao aplicar perfil: %v", err)
		ui.Pause()
		return
	}

	ui.Successf("%s", result.Summary)
	for _, detail := range result.Details {
		ui.Infof("%s", detail)
	}
}

// pickExistingProfile lista os perfis existentes e pede que o usuário
// escolha um. Devolve ok=false se a lista estiver vazia, o usuário
// interromper com Ctrl+C, ou ocorrer um erro (já tratado internamente).
func (s *screen) pickExistingProfile(t tool.Tool, message string) (string, bool) {
	toolID := t.Meta().ID

	names, err := config.List(toolID)
	if err != nil {
		ui.Errorf("erro ao listar perfis: %v", err)
		ui.Pause()
		return "", false
	}

	if len(names) == 0 {
		ui.Warnf("Ainda não há perfis salvos para %s.", t.Meta().Title)
		return "", false
	}

	chosen := ""
	promptErr := survey.AskOne(&survey.Select{
		Message: message,
		Options: names,
	}, &chosen)
	if promptErr != nil {
		if !isInterrupt(promptErr) {
			ui.Errorf("erro ao ler seleção: %v", promptErr)
			ui.Pause()
		}
		return "", false
	}

	return chosen, true
}

// askNewProfileName pergunta um nome de perfil, validando com
// config.ValidateName e repetindo a pergunta em caso de nome inválido, até
// maxNameTries tentativas. Devolve ok=false se o usuário interromper com
// Ctrl+C ou esgotar as tentativas.
func askNewProfileName() (string, bool) {
	for attempt := 0; attempt < maxNameTries; attempt++ {
		name := ""
		err := survey.AskOne(&survey.Input{
			Message: "Nome do novo perfil:",
		}, &name)
		if err != nil {
			if !isInterrupt(err) {
				ui.Errorf("erro ao ler nome do perfil: %v", err)
				ui.Pause()
			}
			return "", false
		}

		if err := config.ValidateName(name); err != nil {
			ui.Errorf("nome inválido: %v", err)
			continue
		}

		return name, true
	}

	ui.Errorf("número máximo de tentativas excedido ao informar o nome do perfil.")
	ui.Pause()
	return "", false
}

// isInterrupt indica se err é (ou envolve) terminal.InterruptErr, sinal de
// que o usuário pressionou Ctrl+C.
func isInterrupt(err error) bool {
	return errors.Is(err, terminal.InterruptErr)
}
