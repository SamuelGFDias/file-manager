package tool

// FlagDoc documenta uma flag para fins de documentação exportável. É
// derivada de Param via DocFlags.
type FlagDoc struct {
	Name        string
	Shorthand   string
	Type        string
	Default     string
	Description string
	Example     string
}

// ExampleDoc documenta um exemplo de uso de uma ferramenta.
type ExampleDoc struct {
	// Title descreve o que o exemplo faz, em português.
	Title string
	// Command é a linha de comando completa do exemplo.
	Command string
}

// Doc é a documentação exportável de uma ferramenta.
type Doc struct {
	ID          string
	Title       string
	// Summary é uma linha resumindo a ferramenta.
	Summary string
	// Description é um parágrafo explicando o comportamento da ferramenta.
	Description string
	// WhenToUse lista gatilhos de uso. Ex: "quando o usuário pedir para ...".
	WhenToUse []string
	Flags     []FlagDoc
	Examples  []ExampleDoc
	// ProfileSchema é um YAML de exemplo do perfil da ferramenta; vazio se
	// a ferramenta não suporta perfis.
	ProfileSchema string
	// Notes lista limitações e avisos importantes sobre a ferramenta.
	Notes []string
}

// DocFlags converte []Param em []FlagDoc, preservando a ordem. Para
// entrada nil ou vazia, devolve um slice vazio não-nil.
func DocFlags(params []Param) []FlagDoc {
	flags := make([]FlagDoc, 0, len(params))
	for _, p := range params {
		flags = append(flags, FlagDoc{
			Name:        p.Name,
			Shorthand:   p.Shorthand,
			Type:        p.Type,
			Default:     p.Default,
			Description: p.Description,
			Example:     p.Example,
		})
	}
	return flags
}
