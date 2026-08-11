// Package tool define o contrato que toda ferramenta do CLI implementa.
//
// A peça central é Param: a declaração única de cada parâmetro de uma
// ferramenta, usada ao mesmo tempo para (a) registrar a flag real do
// cobra/pflag, (b) fazer a pergunta correspondente no modo interativo e
// (c) gerar a documentação exportável. Como as três representações nascem
// da mesma struct, elas não podem divergir entre si.
package tool

import (
	"fmt"

	"github.com/SamuelGFDias/file-manager/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Meta contém os metadados de identificação de uma ferramenta.
type Meta struct {
	// ID é o identificador estável, em kebab-case, igual ao nome do
	// subcomando. Ex: "merge-pdf".
	ID string
	// Title é o rótulo humano exibido no menu interativo. Ex: "Unir PDFs".
	Title string
	// Description é uma descrição de uma linha da ferramenta.
	Description string
}

// Param é a declaração única de um parâmetro de ferramenta. A partir dela
// derivam a flag do cobra (BindFlag), a pergunta do modo interativo
// (Prompt) e a documentação exportável (via DocFlags).
type Param struct {
	// Name é o nome longo da flag, sem o prefixo "--". Ex: "max-depth".
	Name string
	// Shorthand é a letra curta da flag, sem o prefixo "-". Vazio se o
	// parâmetro não tiver forma curta.
	Shorthand string
	// Type é o tipo do parâmetro ("string", "int", "bool", "stringSlice",
	// ...), usado apenas para fins de documentação.
	Type string
	// Description descreve o parâmetro.
	Description string
	// Default é a representação textual do valor padrão, usada na
	// documentação.
	Default string
	// Example é um valor de exemplo, usado na documentação.
	Example string
	// BindFlag registra a flag real no FlagSet informado. Deve fechar
	// sobre o campo correspondente da struct de Options da ferramenta.
	// Pode ser nil quando o parâmetro não é exposto como flag.
	BindFlag func(fs *pflag.FlagSet)
	// Prompt faz a pergunta interativa correspondente e grava o valor no
	// mesmo campo referenciado por BindFlag. Pode ser nil quando o
	// parâmetro não é perguntável no modo interativo.
	Prompt func() error
}

// Result é o resultado da execução de uma ferramenta.
type Result struct {
	// Summary é uma linha resumindo o resultado. Ex: "12 arquivos organizados".
	Summary string
	// Details contém linhas extras opcionais com detalhes do resultado.
	Details []string
}

// ProfileSupport é implementado pelas ferramentas que aceitam perfis
// salvos, permitindo criar, editar e aplicar um conjunto de Options
// nomeado e reutilizável.
type ProfileSupport interface {
	// Empty retorna um ponteiro para uma struct de Options zerada.
	Empty() any
	// Edit reusa as perguntas do modo interativo para editar as Options
	// atuais, retornando a versão editada.
	Edit(nav *ui.Navigator, current any) (any, error)
	// Apply executa a ferramenta com as Options informadas. É a mesma
	// função usada pelo RunE do comando cobra.
	Apply(opts any) (Result, error)
}

// Tool é o contrato que toda ferramenta do CLI implementa.
type Tool interface {
	// Meta retorna os metadados de identificação da ferramenta.
	Meta() Meta
	// Command retorna o subcomando cobra correspondente.
	Command() *cobra.Command
	// Screen retorna a tela do modo interativo correspondente.
	Screen() ui.Screen
	// Doc retorna a documentação exportável da ferramenta.
	Doc() Doc
	// Profile retorna o suporte a perfis salvos da ferramenta, ou nil
	// quando a ferramenta não suporta perfis.
	Profile() ProfileSupport
}

// BindAll registra no FlagSet todas as flags dos params que tenham
// BindFlag != nil. Params sem BindFlag são ignorados silenciosamente.
func BindAll(fs *pflag.FlagSet, params []Param) {
	for _, p := range params {
		if p.BindFlag == nil {
			continue
		}
		p.BindFlag(fs)
	}
}

// PromptAll executa em ordem todos os Prompt != nil, parando no primeiro
// erro encontrado. Params sem Prompt são ignorados silenciosamente. O erro
// retornado é envolvido com o nome do parâmetro que falhou.
func PromptAll(params []Param) error {
	for _, p := range params {
		if p.Prompt == nil {
			continue
		}
		if err := p.Prompt(); err != nil {
			return fmt.Errorf("parâmetro %q: %w", p.Name, err)
		}
	}
	return nil
}
