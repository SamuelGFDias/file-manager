//go:build e2e && linux

package e2e

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Este arquivo é uma cópia adaptada (com o mesmo comportamento e comentários
// de origem) do gerador de PDF de fixture usado por
// internal/pdfutil/integration_test.go (função buildTestPDF). Não foi
// possível reaproveitar o original: ele vive em um arquivo _test.go de
// outro pacote (internal/pdfutil), e arquivos _test.go não são
// importáveis por pacotes externos — nem mesmo dentro do mesmo módulo. A
// alternativa seria promovê-lo a um arquivo não-test exportado dentro de
// internal/pdfutil, o que alteraria código de produção (ainda que só
// acrescentando um símbolo de teste) — fora do escopo permitido aqui. Copiar
// a lógica mínima do gerador para este pacote de teste foi o caminho que
// não exigiu tocar em nenhum arquivo fora de internal/testcli/ e e2e/.

// buildTestPDF monta, byte a byte, um PDF válido mínimo com uma página por
// item de pagesText, cada uma com uma linha de texto extraível. O formato é
// deliberadamente simples (xref clássico, sem compressão, fonte Type1
// Helvetica embutida só por referência ao nome base, sem Widths) — o
// suficiente para abrir tanto no seletor de arquivos (que só olha a
// extensão) quanto, se algum teste precisar, na extração de texto real via
// ledongthuc/pdf.
type pdfBuilder struct {
	buf     bytes.Buffer
	offsets []int // offsets[i] = offset do objeto (i+1) no buffer final
}

func newPDFBuilder() *pdfBuilder {
	b := &pdfBuilder{}
	b.buf.WriteString("%PDF-1.4\n")
	return b
}

func (b *pdfBuilder) reserveID() int {
	id := len(b.offsets) + 1
	b.offsets = append(b.offsets, -1)
	return id
}

func (b *pdfBuilder) placeObj(id int, body string) {
	b.offsets[id-1] = b.buf.Len()
	fmt.Fprintf(&b.buf, "%d 0 obj\n%s\nendobj\n", id, body)
}

func (b *pdfBuilder) addObj(body string) int {
	id := b.reserveID()
	b.placeObj(id, body)
	return id
}

func (b *pdfBuilder) addStreamObj(content []byte) int {
	id := len(b.offsets) + 1
	b.offsets = append(b.offsets, b.buf.Len())
	fmt.Fprintf(&b.buf, "%d 0 obj\n<< /Length %d >>\nstream\n", id, len(content))
	b.buf.Write(content)
	b.buf.WriteString("\nendstream\nendobj\n")
	return id
}

func (b *pdfBuilder) finish(rootID int) []byte {
	xrefOffset := b.buf.Len()
	b.buf.WriteString("xref\n")
	fmt.Fprintf(&b.buf, "0 %d\n", len(b.offsets)+1)
	b.buf.WriteString("0000000000 65535 f \n")
	for _, off := range b.offsets {
		fmt.Fprintf(&b.buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b.buf, "trailer\n<< /Size %d /Root %d 0 R >>\nstartxref\n%d\n%%%%EOF",
		len(b.offsets)+1, rootID, xrefOffset)
	return b.buf.Bytes()
}

// buildTestPDF gera um PDF de len(pagesText) páginas; a página i contém,
// como único conteúdo, uma linha de texto "BT ... (pagesText[i]) Tj ET" na
// posição (72, 720). Os parênteses literais do operador PDF exigem que
// pagesText não contenha "(", ")" ou "\".
func buildTestPDF(t *testing.T, pagesText []string) []byte {
	t.Helper()
	for _, s := range pagesText {
		if strings.ContainsAny(s, "()\\") {
			t.Fatalf("buildTestPDF: texto de página %q contém caractere que precisaria de escape PDF; ajuste o teste", s)
		}
	}

	b := newPDFBuilder()

	catalogID := b.reserveID()
	pagesID := b.reserveID()
	fontID := b.addObj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")

	pageIDs := make([]int, len(pagesText))
	for i, text := range pagesText {
		contentID := b.addStreamObj([]byte(fmt.Sprintf("BT /F1 12 Tf 72 720 Td (%s) Tj ET", text)))
		pageID := b.reserveID()
		pageIDs[i] = pageID
		b.placeObj(pageID, fmt.Sprintf(
			"<< /Type /Page /Parent %d 0 R /Resources << /Font << /F1 %d 0 R >> >> /MediaBox [0 0 612 792] /Contents %d 0 R >>",
			pagesID, fontID, contentID,
		))
	}

	var kids strings.Builder
	for _, id := range pageIDs {
		fmt.Fprintf(&kids, "%d 0 R ", id)
	}
	b.placeObj(pagesID, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids.String(), len(pageIDs)))
	b.placeObj(catalogID, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pagesID))

	return b.finish(catalogID)
}

// writeTestPDF escreve o resultado de buildTestPDF em disco, dentro de dir,
// e devolve o caminho completo.
func writeTestPDF(t *testing.T, dir, name string, pagesText []string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buildTestPDF(t, pagesText), 0o644); err != nil {
		t.Fatalf("escrever PDF de teste %q: %v", path, err)
	}
	return path
}
