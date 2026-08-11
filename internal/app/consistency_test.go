package app

import (
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// zeroDefaultEquivalents são os valores de pflag.Flag.DefValue considerados
// equivalentes a "sem default declarado" (Doc().Flags[i].Default == "").
// Cada tipo de flag usado pelas ferramentas tem sua própria representação de
// zero-value: string vazia, bool falso, int zero e slice vazio.
var zeroDefaultEquivalents = map[string]bool{
	"":      true,
	"false": true,
	"0":     true,
	"[]":    true,
}

// TestToolsConsistency roda, para CADA ferramenta devolvida por Tools(), um
// conjunto de checagens que garantem que Doc(), Meta() e Command() nunca
// divergem entre si. É o teste central que substitui a checagem espalhada
// (e incompleta) que cada ferramenta fazia por conta própria: ao comparar
// só os NOMES de flag entre Doc().Flags e Command().Flags(), uma
// divergência de VALOR PADRÃO documentado passava despercebida (foi
// exatamente o que aconteceu com --name-template em split-pdf).
//
// Por rodar sobre app.Tools() em vez de depender de cada ferramenta se
// testar, uma ferramenta nova entra automaticamente na cobertura assim que
// é registrada em registry.go — ninguém precisa lembrar de escrever este
// teste de novo.
func TestToolsConsistency(t *testing.T) {
	for _, tl := range Tools() {
		meta := tl.Meta()

		t.Run(meta.ID, func(t *testing.T) {
			doc := tl.Doc()
			cmd := tl.Command()
			flags := cmd.Flags()

			// 5. Meta().ID deve ser igual ao Use do comando, ou ao primeiro
			// token de Use quando ele inclui argumentos posicionais (ex:
			// "minha-ferramenta <arquivo>").
			useID := strings.Fields(cmd.Use)[0]
			if useID != meta.ID {
				t.Errorf("ferramenta %q: Command().Use = %q (ID efetivo %q), esperava igual a Meta().ID = %q",
					meta.ID, cmd.Use, useID, meta.ID)
			}

			// 6. Doc().ID deve ser igual a Meta().ID.
			if doc.ID != meta.ID {
				t.Errorf("ferramenta %q: Doc().ID = %q, esperava igual a Meta().ID = %q", meta.ID, doc.ID, meta.ID)
			}

			// 7. Campos essenciais da Doc não podem estar vazios: uma
			// documentação vazia derrota o propósito de exportá-la para IA.
			if strings.TrimSpace(doc.Title) == "" {
				t.Errorf("ferramenta %q: Doc().Title está vazio", meta.ID)
			}
			if strings.TrimSpace(doc.Summary) == "" {
				t.Errorf("ferramenta %q: Doc().Summary está vazio", meta.ID)
			}
			if strings.TrimSpace(doc.Description) == "" {
				t.Errorf("ferramenta %q: Doc().Description está vazia", meta.ID)
			}
			if len(doc.Examples) < 1 {
				t.Errorf("ferramenta %q: Doc().Examples não tem nenhuma entrada, esperava ao menos 1", meta.ID)
			}

			// 8. Todo comando de exemplo deve começar com "file-manager
			// <id-da-ferramenta>". Um exemplo com o comando errado é
			// exatamente o tipo de coisa que faz uma IA gerar um comando
			// inválido a partir da documentação exportada.
			wantPrefix := "file-manager " + meta.ID
			for i, ex := range doc.Examples {
				if !strings.HasPrefix(ex.Command, wantPrefix) {
					t.Errorf("ferramenta %q: Doc().Examples[%d] (%q) tem comando %q, esperava que começasse com %q",
						meta.ID, i, ex.Title, ex.Command, wantPrefix)
				}
			}

			// 1. Todo nome em Doc().Flags precisa existir de fato como flag
			// registrada em Command().Flags().
			docNames := make(map[string]bool, len(doc.Flags))
			for _, fd := range doc.Flags {
				docNames[fd.Name] = true

				f := flags.Lookup(fd.Name)
				if f == nil {
					t.Errorf("ferramenta %q: Doc().Flags declara a flag %q, mas ela não existe em Command().Flags()",
						meta.ID, fd.Name)
					continue
				}

				// 3. O valor default documentado precisa bater com o
				// default real da flag (pflag.Flag.DefValue). Default
				// vazio no Doc é tratado como "sem default declarado" e
				// aceita qualquer zero-value real ("", "false", "0",
				// "[]"); um Default não-vazio no Doc precisa bater
				// EXATAMENTE com DefValue. É este item que pega o bug de
				// --name-template.
				if fd.Default == "" {
					if !zeroDefaultEquivalents[f.DefValue] {
						t.Errorf("ferramenta %q: flag %q não declara Default no Doc, mas o default real da flag é %q (não é um zero-value)",
							meta.ID, fd.Name, f.DefValue)
					}
				} else if fd.Default != f.DefValue {
					t.Errorf("ferramenta %q: flag %q declara Default %q no Doc, mas o default real da flag (DefValue) é %q",
						meta.ID, fd.Name, fd.Default, f.DefValue)
				}

				// 4. O shorthand documentado precisa bater com o
				// shorthand real da flag.
				if fd.Shorthand != f.Shorthand {
					t.Errorf("ferramenta %q: flag %q declara Shorthand %q no Doc, mas o shorthand real da flag é %q",
						meta.ID, fd.Name, fd.Shorthand, f.Shorthand)
				}
			}

			// 2. O inverso do item 1: toda flag registrada em
			// Command().Flags() precisa aparecer em Doc().Flags. Uma flag
			// real não documentada é tão ruim quanto uma documentada que
			// não existe — a IA que ler a documentação nunca vai saber
			// que ela existe.
			flags.VisitAll(func(f *pflag.Flag) {
				if !docNames[f.Name] {
					t.Errorf("ferramenta %q: a flag %q existe em Command().Flags(), mas não está documentada em Doc().Flags",
						meta.ID, f.Name)
				}
			})
		})
	}
}
