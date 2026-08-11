// Package pdfutil implementa o núcleo de manipulação de PDF do CLI:
// união, separação, organização e extração de texto.
package pdfutil

import (
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ExtractPageTexts devolve o texto de cada página do PDF em path.
// O índice 0 do slice retornado corresponde à página 1 do documento.
func ExtractPageTexts(path string) ([]string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("abrir PDF %q: %w", path, err)
	}
	defer f.Close()

	numPages := r.NumPage()
	texts := make([]string, 0, numPages)

	for i := 1; i <= numPages; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			texts = append(texts, "")
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			// Página com falha de extração não deve abortar o documento
			// inteiro: registramos texto vazio e seguimos.
			texts = append(texts, "")
			continue
		}
		texts = append(texts, text)
	}

	return texts, nil
}

// ExtractText devolve o texto do documento inteiro em path, com o texto de
// cada página concatenado por "\n".
func ExtractText(path string) (string, error) {
	pages, err := ExtractPageTexts(path)
	if err != nil {
		return "", err
	}
	return strings.Join(pages, "\n"), nil
}
