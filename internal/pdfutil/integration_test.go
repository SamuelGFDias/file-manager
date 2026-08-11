package pdfutil

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	pdflib "github.com/ledongthuc/pdf"
)

// Este arquivo cobre uma lacuna importante da suíte: até aqui, todos os
// testes de pdfutil trabalham em cima de lógica pura (strings, regex,
// caminhos) — nenhum exercita merge/split/organize contra um PDF de verdade,
// passando pelas bibliotecas pdfcpu (merge/split) e ledongthuc/pdf (extração
// de texto). Os testes abaixo geram fixtures mínimas EM TEMPO DE TESTE (não
// há binário commitado) e as usam ponta a ponta.

// --- construção da fixture -------------------------------------------------
//
// buildTestPDF monta, byte a byte, um PDF válido mínimo com uma página por
// item de pagesText, cada uma com uma linha de texto extraível. O formato é
// deliberadamente simples (xref clássico, sem compressão, fonte Type1
// Helvetica embutida só por referência ao nome base, sem Widths) — é o
// suficiente para que tanto pdfcpu (merge/split, mais rigoroso) quanto
// github.com/ledongthuc/pdf (extração de texto) consigam ler o arquivo.
// Validado manualmente antes de ir para esta suíte: ambas as bibliotecas
// abrem e operam sobre o resultado sem erro.
type pdfBuilder struct {
	buf     bytes.Buffer
	offsets []int // offsets[i] = offset do objeto (i+1) no buffer final
}

func newPDFBuilder() *pdfBuilder {
	b := &pdfBuilder{}
	b.buf.WriteString("%PDF-1.4\n")
	return b
}

// reserveID reserva o próximo número de objeto sem escrevê-lo ainda —
// necessário quando um objeto precisa referenciar outro que só existe depois
// dele no arquivo (ex: Page referenciando Pages, que só é escrito no final).
func (b *pdfBuilder) reserveID() int {
	id := len(b.offsets) + 1
	b.offsets = append(b.offsets, -1)
	return id
}

// placeObj escreve o corpo de um objeto previamente reservado com reserveID.
func (b *pdfBuilder) placeObj(id int, body string) {
	b.offsets[id-1] = b.buf.Len()
	fmt.Fprintf(&b.buf, "%d 0 obj\n%s\nendobj\n", id, body)
}

// addObj reserva e escreve um objeto de uma vez, para quando não há
// dependência circular de referências.
func (b *pdfBuilder) addObj(body string) int {
	id := b.reserveID()
	b.placeObj(id, body)
	return id
}

// addStreamObj escreve um objeto stream (usado para o conteúdo de cada
// página).
func (b *pdfBuilder) addStreamObj(content []byte) int {
	id := len(b.offsets) + 1
	b.offsets = append(b.offsets, b.buf.Len())
	fmt.Fprintf(&b.buf, "%d 0 obj\n<< /Length %d >>\nstream\n", id, len(content))
	b.buf.Write(content)
	b.buf.WriteString("\nendstream\nendobj\n")
	return id
}

// finish escreve a tabela xref clássica e o trailer, fechando o arquivo, e
// devolve os bytes completos do PDF.
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

// buildTestPDF gera um PDF de len(pagesText) páginas; a página i contém, como
// único conteúdo, uma linha de texto "BT ... (pagesText[i]) Tj ET" na posição
// (72, 720). Os parênteses literais do operador PDF exigem que pagesText não
// contenha "(", ")" ou "\" — nenhum dos textos usados nestes testes precisa
// disso.
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

// writeTestPDF escreve o resultado de buildTestPDF em disco, dentro de dir, e
// devolve o caminho completo.
func writeTestPDF(t *testing.T, dir, name string, pagesText []string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buildTestPDF(t, pagesText), 0o644); err != nil {
		t.Fatalf("escrever PDF de teste %q: %v", path, err)
	}
	return path
}

// TestIntegrationFixtureTextIsExtractable valida a premissa de que todos os
// outros testes deste arquivo dependem: o PDF gerado por buildTestPDF tem
// camada de texto extraível pela biblioteca github.com/ledongthuc/pdf (a
// mesma usada por ExtractPageTexts/ExtractText). Se esta asserção falhar, o
// gerador está incorreto — os demais testes de integração pressupõem que ela
// passa.
func TestIntegrationFixtureTextIsExtractable(t *testing.T) {
	dir := t.TempDir()
	path := writeTestPDF(t, dir, "fixture.pdf", []string{"NF: 00123"})

	f, r, err := pdflib.Open(path)
	if err != nil {
		t.Fatalf("pdflib.Open: %v", err)
	}
	defer f.Close()

	if r.NumPage() != 1 {
		t.Fatalf("NumPage() = %d, esperava 1", r.NumPage())
	}
	page := r.Page(1)
	text, err := page.GetPlainText(nil)
	if err != nil {
		t.Fatalf("GetPlainText: %v", err)
	}
	if !strings.Contains(text, "NF: 00123") {
		t.Fatalf("texto extraído %q não contém o marcador esperado \"NF: 00123\"", text)
	}
}

// TestIntegrationMergeSumsPageCount une dois PDFs reais e confere, via
// api.PageCountFile (a mesma função usada internamente por pdfutil.Merge),
// que o total de páginas do arquivo resultante é a soma das entradas.
func TestIntegrationMergeSumsPageCount(t *testing.T) {
	dir := t.TempDir()
	pdfA := writeTestPDF(t, dir, "a.pdf", []string{"pagina 1 de a", "pagina 2 de a"})
	pdfB := writeTestPDF(t, dir, "b.pdf", []string{"pagina 1 de b"})

	output := filepath.Join(dir, "merged.pdf")
	result, err := Merge(context.Background(), MergeOptions{
		Inputs: []string{pdfA, pdfB},
		Output: output,
	})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	if result.PageCount != 3 {
		t.Fatalf("PageCount = %d, esperava 3 (2 + 1)", result.PageCount)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("arquivo de saída não foi criado: %v", err)
	}
}

// TestIntegrationSplitByPageGeneratesOneFilePerPage separa um PDF real de 3
// páginas no modo "page" e confere que exatamente 3 arquivos foram gerados.
func TestIntegrationSplitByPageGeneratesOneFilePerPage(t *testing.T) {
	dir := t.TempDir()
	input := writeTestPDF(t, dir, "entrada.pdf", []string{"pagina um", "pagina dois", "pagina tres"})
	outDir := filepath.Join(dir, "saida")

	result, err := Split(context.Background(), SplitOptions{
		Input:     input,
		OutputDir: outDir,
		Mode:      SplitByPage,
	})
	if err != nil {
		t.Fatalf("Split (page): %v", err)
	}

	if len(result.Outputs) != 3 {
		t.Fatalf("gerou %d arquivos, esperava 3: %v", len(result.Outputs), result.Outputs)
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("ler outDir: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("outDir contém %d arquivos, esperava 3", len(entries))
	}
}

// TestIntegrationSplitByRegexNamesFileByCaptureNoDoubleExtension é a
// regressão de integração do defeito relatado (pagina-001.pdf.pdf): separa
// um PDF real cujo texto casa com "NF:\s*(\d+)" e confere que o arquivo
// gerado se chama exatamente "00123.pdf" — nem "00123" sem extensão, nem
// "00123.pdf.pdf".
func TestIntegrationSplitByRegexNamesFileByCaptureNoDoubleExtension(t *testing.T) {
	dir := t.TempDir()
	input := writeTestPDF(t, dir, "notas.pdf", []string{"NF: 00123", "conteudo da nota"})
	outDir := filepath.Join(dir, "saida")

	result, err := Split(context.Background(), SplitOptions{
		Input:     input,
		OutputDir: outDir,
		Mode:      SplitByRegex,
		Regex:     regexp.MustCompile(`NF:\s*(\d+)`),
	})
	if err != nil {
		t.Fatalf("Split (regex): %v", err)
	}

	if len(result.Outputs) != 1 {
		t.Fatalf("gerou %d arquivos, esperava 1: %v", len(result.Outputs), result.Outputs)
	}
	gotName := filepath.Base(result.Outputs[0])
	if gotName != "00123.pdf" {
		t.Fatalf("nome do arquivo gerado = %q, esperava %q (sem extensão duplicada)", gotName, "00123.pdf")
	}
}

// TestIntegrationSplitByRangeUserTemplateWithExtensionNoDoubleExtension
// cobre o modo "range" com um --name-template fornecido pelo usuário que já
// termina em ".pdf" (situação que o usuário pode escolher livremente, sem
// saber da convenção interna de que os templates padrão não trazem
// extensão): o arquivo final não pode duplicar a extensão.
func TestIntegrationSplitByRangeUserTemplateWithExtensionNoDoubleExtension(t *testing.T) {
	dir := t.TempDir()
	input := writeTestPDF(t, dir, "entrada.pdf", []string{"pagina um", "pagina dois", "pagina tres", "pagina quatro"})
	outDir := filepath.Join(dir, "saida")

	result, err := Split(context.Background(), SplitOptions{
		Input:        input,
		OutputDir:    outDir,
		Mode:         SplitByRange,
		Ranges:       []string{"1-2", "3-4"},
		NameTemplate: "intervalo-%03d.pdf", // usuário já incluiu a extensão
	})
	if err != nil {
		t.Fatalf("Split (range): %v", err)
	}

	want := []string{"intervalo-001.pdf", "intervalo-002.pdf"}
	if len(result.Outputs) != len(want) {
		t.Fatalf("gerou %d arquivos, esperava %d: %v", len(result.Outputs), len(want), result.Outputs)
	}
	for i, w := range want {
		got := filepath.Base(result.Outputs[i])
		if got != w {
			t.Errorf("arquivo %d = %q, esperava %q (sem extensão duplicada)", i, got, w)
		}
	}
}

// TestIntegrationExtractPageTexts confere que ExtractPageTexts devolve o
// texto esperado de cada página de um PDF real, na ordem correta.
func TestIntegrationExtractPageTexts(t *testing.T) {
	dir := t.TempDir()
	input := writeTestPDF(t, dir, "entrada.pdf", []string{"primeira pagina", "segunda pagina"})

	texts, err := ExtractPageTexts(input)
	if err != nil {
		t.Fatalf("ExtractPageTexts: %v", err)
	}
	if len(texts) != 2 {
		t.Fatalf("ExtractPageTexts devolveu %d páginas, esperava 2", len(texts))
	}
	if !strings.Contains(texts[0], "primeira pagina") {
		t.Errorf("texto da página 1 = %q, esperava conter %q", texts[0], "primeira pagina")
	}
	if !strings.Contains(texts[1], "segunda pagina") {
		t.Errorf("texto da página 2 = %q, esperava conter %q", texts[1], "segunda pagina")
	}
}

// TestIntegrationOrganizeCopiesNonDestructively organiza uma pasta com um PDF
// real usando um nível de regex e confere que: (a) o arquivo é copiado para
// a pasta de destino calculada a partir do texto extraído; (b) o arquivo
// original permanece na pasta de origem — comportamento não-destrutivo do
// default (Copy, não Move).
func TestIntegrationOrganizeCopiesNonDestructively(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	srcPath := filepath.Join(inputDir, "documento.pdf")
	if err := os.WriteFile(srcPath, buildTestPDF(t, []string{"Empresa: Acme\nNF: 00789"}), 0o644); err != nil {
		t.Fatalf("escrever PDF de teste: %v", err)
	}

	result, err := Organize(context.Background(), OrganizeOptions{
		InputDir:  inputDir,
		OutputDir: outputDir,
		Levels: []Level{
			{Label: "empresa", Regex: regexp.MustCompile(`Empresa: (\w+)`)},
		},
		FilenameRegex: regexp.MustCompile(`NF:\s*(\d+)`),
		Copy:          true,
	})
	if err != nil {
		t.Fatalf("Organize: %v", err)
	}

	if len(result.Organized) != 1 {
		t.Fatalf("esperava 1 arquivo organizado, obteve %d (unclassified: %+v)", len(result.Organized), result.Unclassified)
	}
	wantDest := filepath.Join("Acme", "00789.pdf")
	if result.Organized[0].Dest != wantDest {
		t.Fatalf("Dest = %q, esperava %q", result.Organized[0].Dest, wantDest)
	}

	destAbs := filepath.Join(outputDir, wantDest)
	if _, err := os.Stat(destAbs); err != nil {
		t.Fatalf("arquivo de destino não foi criado em %q: %v", destAbs, err)
	}

	// Não-destrutivo: o arquivo original continua na origem.
	if _, err := os.Stat(srcPath); err != nil {
		t.Fatalf("arquivo original deveria continuar em %q (Copy, não Move): %v", srcPath, err)
	}
}
