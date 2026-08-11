// Command file-manager é o binário do CLI: utilitário de manipulação de
// arquivos com foco em operações sobre PDFs (unir, separar, organizar).
//
// Toda a lógica de montagem do comando raiz e execução vive em
// internal/app; main apenas injeta as informações de versão (definidas em
// tempo de build via -ldflags -X) e traduz o código de saída devolvido por
// app.Execute em uma chamada a os.Exit.
package main

import (
	"os"

	"github.com/SamuelGFDias/file-manager/internal/app"
)

// version, commit e date são injetadas em tempo de build via
// -ldflags "-X main.version=... -X main.commit=... -X main.date=...".
// Mantenha os nomes exatos: o processo de build depende deles.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(app.Execute(app.Version{Version: version, Commit: commit, Date: date}))
}
