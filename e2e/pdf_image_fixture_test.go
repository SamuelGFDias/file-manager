//go:build e2e && linux

package e2e

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Este arquivo é uma cópia adaptada (mesmo raciocínio de pdf_fixture_test.go
// — arquivos _test.go não são importáveis entre pacotes, nem mesmo dentro
// do mesmo módulo) do gerador de PDF com imagem embutida usado por
// internal/pdfutil/organize_test.go (imgPDFBuilder/buildImagePDF). Serve só
// ao cenário e2e de ocr-pdf (TestOCRPdfDryRunClassifiesEligibleAndSkipped),
// que precisa de um PDF "puro scan" de verdade — uma imagem raster
// embutida via /XObject, sem nenhum texto — para que a classificação de
// elegibilidade de ocr-pdf tenha algo real para aceitar.

type imgPDFBuilder struct {
	buf     bytes.Buffer
	offsets []int
}

func newImgPDFBuilder() *imgPDFBuilder {
	b := &imgPDFBuilder{}
	b.buf.WriteString("%PDF-1.4\n")
	return b
}

func (b *imgPDFBuilder) reserveID() int {
	id := len(b.offsets) + 1
	b.offsets = append(b.offsets, -1)
	return id
}

func (b *imgPDFBuilder) placeObj(id int, body string) {
	b.offsets[id-1] = b.buf.Len()
	fmt.Fprintf(&b.buf, "%d 0 obj\n%s\nendobj\n", id, body)
}

// addRawStreamObj escreve um objeto stream sem filtro (dictExtra some às
// entradas fixas do dicionário; content é o conteúdo cru do stream).
func (b *imgPDFBuilder) addRawStreamObj(dictExtra string, content []byte) int {
	id := len(b.offsets) + 1
	b.offsets = append(b.offsets, b.buf.Len())
	fmt.Fprintf(&b.buf, "%d 0 obj\n<< %s /Length %d >>\nstream\n", id, dictExtra, len(content))
	b.buf.Write(content)
	b.buf.WriteString("\nendstream\nendobj\n")
	return id
}

func (b *imgPDFBuilder) finish(rootID int) []byte {
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

// buildImagePDF gera um PDF de uma página com uma imagem raster 2x2 em
// escala de cinza (sem camada de texto embutida) embutida via /XObject —
// um PDF "puro scan" de verdade, o único caso que ocr-pdf aceita
// processar.
func buildImagePDF(t *testing.T) []byte {
	t.Helper()
	b := newImgPDFBuilder()

	catalogID := b.reserveID()
	pagesID := b.reserveID()

	imgData := []byte{0x00, 0x40, 0x80, 0xFF}
	imgID := b.addRawStreamObj("/Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8", imgData)

	contentID := b.addRawStreamObj("", []byte("q 50 0 0 50 10 10 cm /Im1 Do Q"))

	pageID := b.reserveID()
	b.placeObj(pageID, fmt.Sprintf(
		"<< /Type /Page /Parent %d 0 R /Resources << /XObject << /Im1 %d 0 R >> >> /MediaBox [0 0 612 792] /Contents %d 0 R >>",
		pagesID, imgID, contentID,
	))

	b.placeObj(pagesID, fmt.Sprintf("<< /Type /Pages /Kids [%d 0 R] /Count 1 >>", pageID))
	b.placeObj(catalogID, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pagesID))

	return b.finish(catalogID)
}

// writeImagePDF escreve o resultado de buildImagePDF em disco, dentro de
// dir, e devolve o caminho completo.
func writeImagePDF(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buildImagePDF(t), 0o644); err != nil {
		t.Fatalf("escrever PDF de teste com imagem %q: %v", path, err)
	}
	return path
}
