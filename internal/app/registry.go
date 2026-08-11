// Package app monta a "amarração final" do CLI file-manager: o registro
// central de ferramentas, o comando raiz do cobra e o ponto de entrada
// chamado pelo main.
package app

import (
	"github.com/SamuelGFDias/file-manager/internal/tool"
	"github.com/SamuelGFDias/file-manager/internal/tools/mergepdf"
	"github.com/SamuelGFDias/file-manager/internal/tools/ocrpdf"
	"github.com/SamuelGFDias/file-manager/internal/tools/organizepdf"
	"github.com/SamuelGFDias/file-manager/internal/tools/splitpdf"
)

// Tools devolve todas as ferramentas registradas no CLI, na ordem em que
// aparecem no menu interativo e como subcomandos do cobra.
//
// PONTO DE REGISTRO: para acrescentar uma ferramenta nova ao CLI, adicione
// uma linha aqui (o gerador em cmd/scaffold instrui exatamente isso ao
// final da execução). Nenhum outro arquivo precisa ser tocado para que uma
// ferramenta nova apareça no menu, nos subcomandos e na documentação
// exportável.
func Tools() []tool.Tool {
	return []tool.Tool{
		mergepdf.New(),
		splitpdf.New(),
		organizepdf.New(),
		ocrpdf.New(),
	}
}
