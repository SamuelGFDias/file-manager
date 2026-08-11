package pdfutil

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func writeCSVFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("escrever planilha de teste %q: %v", path, err)
	}
	return path
}

func TestLoadCSVMapCommaSeparator(t *testing.T) {
	dir := t.TempDir()
	path := writeCSVFile(t, dir, "planilha.csv",
		"NOTA,CIDADE,BAIRRO\n001,Sao Goncalo,Laranjal\n003,Rio de Janeiro,Centro\n")

	m, err := LoadCSVMap(path, "", nil)
	if err != nil {
		t.Fatalf("LoadCSVMap: %v", err)
	}
	if m.KeyColumn != "NOTA" {
		t.Errorf("KeyColumn = %q, esperava %q", m.KeyColumn, "NOTA")
	}
	wantLevels := []string{"CIDADE", "BAIRRO"}
	if !equalStrings(m.Levels, wantLevels) {
		t.Errorf("Levels = %v, esperava %v", m.Levels, wantLevels)
	}
	if len(m.Rows) != 2 {
		t.Fatalf("Rows tem %d entradas, esperava 2: %+v", len(m.Rows), m.Rows)
	}
	got, ok := m.Lookup("001")
	if !ok {
		t.Fatal("chave 001 deveria existir")
	}
	want := []string{"Sao_Goncalo", "Laranjal"}
	if !equalStrings(got, want) {
		t.Errorf("Lookup(001) = %v, esperava %v", got, want)
	}
}

func TestLoadCSVMapSemicolonSeparator(t *testing.T) {
	dir := t.TempDir()
	// Excel em português salva CSV com ";" por padrão — a planilha do
	// enunciado tem valores acentuados, exatamente o caso motivador.
	path := writeCSVFile(t, dir, "planilha.csv",
		"NOTA;CIDADE;BAIRRO\n001;São Gonçalo;Laranjal\n003;Rio de Janeiro;Centro\n005;Niterói;Fonseca\n")

	m, err := LoadCSVMap(path, "", nil)
	if err != nil {
		t.Fatalf("LoadCSVMap: %v", err)
	}
	if len(m.Rows) != 3 {
		t.Fatalf("Rows tem %d entradas, esperava 3: %+v", len(m.Rows), m.Rows)
	}
	got, ok := m.Lookup("001")
	if !ok {
		t.Fatal("chave 001 deveria existir")
	}
	want := []string{"Sao_Goncalo", "Laranjal"}
	if !equalStrings(got, want) {
		t.Errorf("Lookup(001) = %v, esperava %v", got, want)
	}

	got, ok = m.Lookup("005")
	if !ok {
		t.Fatal("chave 005 deveria existir")
	}
	want = []string{"Niteroi", "Fonseca"}
	if !equalStrings(got, want) {
		t.Errorf("Lookup(005) = %v, esperava %v", got, want)
	}
}

func TestLoadCSVMapWithBOM(t *testing.T) {
	dir := t.TempDir()
	content := csvUTF8BOM + "NOTA,CIDADE\n001,Acme\n"
	path := writeCSVFile(t, dir, "planilha.csv", content)

	m, err := LoadCSVMap(path, "", nil)
	if err != nil {
		t.Fatalf("LoadCSVMap: %v", err)
	}
	if m.KeyColumn != "NOTA" {
		t.Fatalf("KeyColumn = %q, esperava %q (BOM deveria ter sido descartado)", m.KeyColumn, "NOTA")
	}
	if _, ok := m.Lookup("001"); !ok {
		t.Fatal("chave 001 deveria existir")
	}
}

func TestLoadCSVMapWithCRLF(t *testing.T) {
	dir := t.TempDir()
	content := "NOTA,CIDADE\r\n001,Acme\r\n003,Beta\r\n"
	path := writeCSVFile(t, dir, "planilha.csv", content)

	m, err := LoadCSVMap(path, "", nil)
	if err != nil {
		t.Fatalf("LoadCSVMap: %v", err)
	}
	if len(m.Rows) != 2 {
		t.Fatalf("Rows tem %d entradas, esperava 2: %+v", len(m.Rows), m.Rows)
	}
}

func TestLoadCSVMapKeyColumnByName(t *testing.T) {
	dir := t.TempDir()
	path := writeCSVFile(t, dir, "planilha.csv", "CIDADE,NOTA,BAIRRO\nSao Goncalo,001,Laranjal\n")

	m, err := LoadCSVMap(path, "NOTA", nil)
	if err != nil {
		t.Fatalf("LoadCSVMap: %v", err)
	}
	if m.KeyColumn != "NOTA" {
		t.Fatalf("KeyColumn = %q, esperava %q", m.KeyColumn, "NOTA")
	}
	wantLevels := []string{"CIDADE", "BAIRRO"}
	if !equalStrings(m.Levels, wantLevels) {
		t.Fatalf("Levels = %v, esperava %v", m.Levels, wantLevels)
	}
	got, ok := m.Lookup("001")
	if !ok {
		t.Fatal("chave 001 deveria existir")
	}
	want := []string{"Sao_Goncalo", "Laranjal"}
	if !equalStrings(got, want) {
		t.Errorf("Lookup(001) = %v, esperava %v", got, want)
	}
}

func TestLoadCSVMapKeyColumnNotFound(t *testing.T) {
	dir := t.TempDir()
	path := writeCSVFile(t, dir, "planilha.csv", "NOTA,CIDADE\n001,Acme\n")

	_, err := LoadCSVMap(path, "FORNECEDOR", nil)
	if err == nil {
		t.Fatal("esperava erro para coluna-chave inexistente")
	}
	if !strings.Contains(err.Error(), "FORNECEDOR") || !strings.Contains(err.Error(), "NOTA") {
		t.Fatalf("mensagem de erro %q deveria citar a coluna pedida e listar as disponíveis", err.Error())
	}
}

func TestLoadCSVMapLevelColumnsSelectedAndReordered(t *testing.T) {
	dir := t.TempDir()
	path := writeCSVFile(t, dir, "planilha.csv", "NOTA,CIDADE,BAIRRO,UF\n001,Sao Goncalo,Laranjal,RJ\n")

	m, err := LoadCSVMap(path, "", []string{"BAIRRO", "UF"})
	if err != nil {
		t.Fatalf("LoadCSVMap: %v", err)
	}
	wantLevels := []string{"BAIRRO", "UF"}
	if !equalStrings(m.Levels, wantLevels) {
		t.Fatalf("Levels = %v, esperava %v (ordem informada respeitada)", m.Levels, wantLevels)
	}
	got, ok := m.Lookup("001")
	if !ok {
		t.Fatal("chave 001 deveria existir")
	}
	want := []string{"Laranjal", "RJ"}
	if !equalStrings(got, want) {
		t.Errorf("Lookup(001) = %v, esperava %v", got, want)
	}
}

func TestLoadCSVMapLevelColumnNotFound(t *testing.T) {
	dir := t.TempDir()
	path := writeCSVFile(t, dir, "planilha.csv", "NOTA,CIDADE\n001,Acme\n")

	_, err := LoadCSVMap(path, "", []string{"FILIAL"})
	if err == nil {
		t.Fatal("esperava erro para coluna de nível inexistente")
	}
	if !strings.Contains(err.Error(), "FILIAL") {
		t.Fatalf("mensagem de erro %q deveria citar a coluna pedida", err.Error())
	}
}

func TestLoadCSVMapDuplicateKeyIsError(t *testing.T) {
	dir := t.TempDir()
	path := writeCSVFile(t, dir, "planilha.csv", "NOTA,CIDADE\n001,Acme\n001,Beta\n")

	_, err := LoadCSVMap(path, "", nil)
	if err == nil {
		t.Fatal("esperava erro por chave duplicada")
	}
	if !strings.Contains(err.Error(), "001") {
		t.Fatalf("mensagem de erro %q deveria citar a chave repetida", err.Error())
	}
}

func TestLoadCSVMapEmptyLevelCellIsWarningNotError(t *testing.T) {
	dir := t.TempDir()
	path := writeCSVFile(t, dir, "planilha.csv", "NOTA,CIDADE,BAIRRO\n001,Sao Goncalo,\n003,,Centro\n")

	m, err := LoadCSVMap(path, "", nil)
	if err != nil {
		t.Fatalf("célula de nível vazia não deveria ser erro: %v", err)
	}
	if len(m.Warnings) != 2 {
		t.Fatalf("esperava 2 avisos (um por célula vazia), obteve %d: %v", len(m.Warnings), m.Warnings)
	}
	for _, w := range m.Warnings {
		if !strings.Contains(w, "001") && !strings.Contains(w, "003") {
			t.Errorf("aviso %q deveria citar a chave da linha", w)
		}
	}

	got, ok := m.Lookup("001")
	if !ok {
		t.Fatal("chave 001 deveria existir mesmo com célula de nível vazia")
	}
	if !equalStrings(got, []string{"Sao_Goncalo"}) {
		t.Errorf("Lookup(001) = %v, esperava só o componente não vazio", got)
	}

	got, ok = m.Lookup("003")
	if !ok {
		t.Fatal("chave 003 deveria existir")
	}
	if !equalStrings(got, []string{"Centro"}) {
		t.Errorf("Lookup(003) = %v, esperava só o componente não vazio", got)
	}
}

func TestLoadCSVMapMissingHeaderOrTooFewColumns(t *testing.T) {
	dir := t.TempDir()

	empty := writeCSVFile(t, dir, "vazio.csv", "")
	if _, err := LoadCSVMap(empty, "", nil); err == nil {
		t.Error("arquivo vazio deveria dar erro")
	}

	umaColuna := writeCSVFile(t, dir, "uma-coluna.csv", "NOTA\n001\n")
	if _, err := LoadCSVMap(umaColuna, "", nil); err == nil {
		t.Error("cabeçalho com 1 coluna só deveria dar erro (falta ao menos um nível)")
	}
}

func TestLoadCSVMapSeparatorAmbiguousIsError(t *testing.T) {
	dir := t.TempDir()
	// Nem vírgula nem ponto e vírgula presentes: nenhum separador detectável.
	path := writeCSVFile(t, dir, "planilha.csv", "NOTA\n001\n")
	_, err := LoadCSVMap(path, "", nil)
	if err == nil {
		t.Fatal("esperava erro de detecção de separador")
	}
}

func TestLoadCSVMapKeyTrimmedMatchesTrimmedPDFKey(t *testing.T) {
	dir := t.TempDir()
	path := writeCSVFile(t, dir, "planilha.csv", "NOTA,CIDADE\n  001  ,Acme\n")

	m, err := LoadCSVMap(path, "", nil)
	if err != nil {
		t.Fatalf("LoadCSVMap: %v", err)
	}
	// A chave extraída de um PDF pode vir com espaços ao redor também.
	if _, ok := m.Lookup("  001  "); !ok {
		t.Fatal("Lookup deveria casar mesmo com espaços nas pontas da chave consultada")
	}
	if _, ok := m.Lookup("001"); !ok {
		t.Fatal("Lookup deveria casar a chave já sem espaços")
	}
}

func TestLoadCSVMapLeadingZerosAreSignificant(t *testing.T) {
	dir := t.TempDir()
	path := writeCSVFile(t, dir, "planilha.csv", "NOTA,CIDADE\n001,Acme\n1,Beta\n")

	m, err := LoadCSVMap(path, "", nil)
	if err != nil {
		t.Fatalf("LoadCSVMap: %v", err)
	}
	got001, ok := m.Lookup("001")
	if !ok {
		t.Fatal("chave 001 deveria existir")
	}
	got1, ok := m.Lookup("1")
	if !ok {
		t.Fatal("chave 1 deveria existir, distinta de 001")
	}
	if equalStrings(got001, got1) {
		t.Fatalf("001 e 1 deveriam ser chaves distintas com valores diferentes, ambas resolveram para %v", got001)
	}
}

func TestLoadCSVMapBlankLinesIgnored(t *testing.T) {
	dir := t.TempDir()
	path := writeCSVFile(t, dir, "planilha.csv", "NOTA,CIDADE\n001,Acme\n\n003,Beta\n")

	m, err := LoadCSVMap(path, "", nil)
	if err != nil {
		t.Fatalf("LoadCSVMap: %v", err)
	}
	if len(m.Rows) != 2 {
		t.Fatalf("Rows tem %d entradas, esperava 2 (linha em branco ignorada): %+v", len(m.Rows), m.Rows)
	}
}

func TestNormalizeComponent(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"São Gonçalo", "Sao_Goncalo"},
		{"Niterói", "Niteroi"},
		{"Rio de Janeiro", "Rio_de_Janeiro"},
		{"", "sem-valor"},
		{"áéíóú", "aeiou"},
	}
	for _, c := range cases {
		got := NormalizeComponent(c.in)
		if got != c.want {
			t.Errorf("NormalizeComponent(%q) = %q, esperava %q", c.in, got, c.want)
		}
	}

	// "a/b" não pode conter separador de caminho no resultado.
	if got := NormalizeComponent("a/b"); strings.ContainsAny(got, `/\`) {
		t.Errorf("NormalizeComponent(\"a/b\") = %q, não deveria conter separador de caminho", got)
	}

	// ".." precisa ser neutralizado (nunca sobreviver intacto no resultado).
	if got := NormalizeComponent(".."); strings.Contains(got, "..") {
		t.Errorf("NormalizeComponent(\"..\") = %q, não deveria conter \"..\"", got)
	}
}

func TestResolveDestinationCSVKeyFound(t *testing.T) {
	m := CSVMap{
		KeyColumn: "NOTA",
		Levels:    []string{"CIDADE", "BAIRRO"},
		Rows: map[string][]string{
			"001": {"Sao_Goncalo", "Laranjal"},
		},
	}
	keyRegex := regexp.MustCompile(`NOTA:\s*(\d+)`)
	text := "NOTA: 001"

	relPath, unmatched := ResolveDestinationCSV(text, m, keyRegex, nil)
	if unmatched != nil {
		t.Fatalf("não esperava falha: %+v", unmatched)
	}
	want := filepath.Join("Sao_Goncalo", "Laranjal", "001.pdf")
	if relPath != want {
		t.Fatalf("relPath = %q, esperava %q", relPath, want)
	}
}

func TestResolveDestinationCSVKeyNotFoundInSheet(t *testing.T) {
	m := CSVMap{Rows: map[string][]string{"001": {"Acme"}}}
	keyRegex := regexp.MustCompile(`NOTA:\s*(\d+)`)
	text := "NOTA: 999"

	_, unmatched := ResolveDestinationCSV(text, m, keyRegex, nil)
	if unmatched == nil {
		t.Fatal("esperava não-classificado: chave 999 não está na planilha")
	}
	if unmatched.Level != "chave" {
		t.Fatalf("Level = %q, esperava %q", unmatched.Level, "chave")
	}
	if !strings.Contains(unmatched.Pattern, "999") {
		t.Fatalf("motivo %q deveria citar a chave encontrada (999)", unmatched.Pattern)
	}
}

func TestResolveDestinationCSVKeyRegexDoesNotMatch(t *testing.T) {
	m := CSVMap{Rows: map[string][]string{"001": {"Acme"}}}
	keyRegex := regexp.MustCompile(`NOTA:\s*(\d+)`)
	text := "documento sem chave nenhuma"

	_, unmatched := ResolveDestinationCSV(text, m, keyRegex, nil)
	if unmatched == nil {
		t.Fatal("esperava não-classificado: regex não casou")
	}
	if unmatched.Level != "chave" {
		t.Fatalf("Level = %q, esperava %q", unmatched.Level, "chave")
	}
	if !strings.Contains(unmatched.Pattern, "não encontrada") {
		t.Fatalf("motivo %q deveria dizer que a chave não foi encontrada no documento", unmatched.Pattern)
	}
}

func TestResolveDestinationCSVFilenameIsKeyByDefault(t *testing.T) {
	m := CSVMap{Rows: map[string][]string{"001": {"Acme"}}}
	keyRegex := regexp.MustCompile(`NOTA:\s*(\d+)`)
	text := "NOTA: 001"

	relPath, unmatched := ResolveDestinationCSV(text, m, keyRegex, nil)
	if unmatched != nil {
		t.Fatalf("não esperava falha: %+v", unmatched)
	}
	if filepath.Base(relPath) != "001.pdf" {
		t.Fatalf("nome do arquivo = %q, esperava a própria chave (001.pdf)", filepath.Base(relPath))
	}
}

func TestResolveDestinationCSVFilenameRegexOverridesKey(t *testing.T) {
	m := CSVMap{Rows: map[string][]string{"001": {"Acme"}}}
	keyRegex := regexp.MustCompile(`NOTA:\s*(\d+)`)
	filenameRegex := regexp.MustCompile(`ARQ:\s*(\w+)`)
	text := "NOTA: 001\nARQ: relatoriofinal"

	relPath, unmatched := ResolveDestinationCSV(text, m, keyRegex, filenameRegex)
	if unmatched != nil {
		t.Fatalf("não esperava falha: %+v", unmatched)
	}
	if filepath.Base(relPath) != "relatoriofinal.pdf" {
		t.Fatalf("nome do arquivo = %q, esperava o valor de FilenameRegex", filepath.Base(relPath))
	}
}

// TestOrganizeCSVModeClassifiesByKey confere o caminho feliz de Organize em
// modo --csv: a chave encontrada no PDF resolve, via a planilha, para a
// hierarquia esperada.
func TestOrganizeCSVModeClassifiesByKey(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(inputDir, "doc.pdf"), buildTestPDF(t, []string{"NOTA: 001"}), 0o644); err != nil {
		t.Fatalf("escrever pdf: %v", err)
	}

	csvMap := CSVMap{
		Rows: map[string][]string{
			"001": {"Sao_Goncalo", "Laranjal"},
		},
	}

	result, err := Organize(context.Background(), OrganizeOptions{
		InputDir:    inputDir,
		OutputDir:   outputDir,
		CSV:         &csvMap,
		CSVKeyRegex: regexp.MustCompile(`NOTA:\s*(\d+)`),
		Copy:        true,
	})
	if err != nil {
		t.Fatalf("Organize: %v", err)
	}
	if len(result.Organized) != 1 {
		t.Fatalf("esperava 1 organizado, obteve %d (unclassified: %+v)", len(result.Organized), result.Unclassified)
	}
	wantDest := filepath.Join("Sao_Goncalo", "Laranjal", "001.pdf")
	if result.Organized[0].Dest != wantDest {
		t.Fatalf("Dest = %q, esperava %q", result.Organized[0].Dest, wantDest)
	}
	if _, err := os.Stat(filepath.Join(outputDir, wantDest)); err != nil {
		t.Fatalf("arquivo de destino não foi criado: %v", err)
	}
}

// TestOrganizeCSVModeKeyMissingFromSheetIsUnclassified confere o caso mais
// comum na prática: uma chave que o regex encontra no PDF, mas que não
// existe na planilha — o arquivo vai para não-classificados citando a
// chave, em vez de interromper o lote.
func TestOrganizeCSVModeKeyMissingFromSheetIsUnclassified(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(inputDir, "doc.pdf"), buildTestPDF(t, []string{"NOTA: 999"}), 0o644); err != nil {
		t.Fatalf("escrever pdf: %v", err)
	}

	csvMap := CSVMap{Rows: map[string][]string{"001": {"Acme"}}}

	result, err := Organize(context.Background(), OrganizeOptions{
		InputDir:    inputDir,
		OutputDir:   outputDir,
		CSV:         &csvMap,
		CSVKeyRegex: regexp.MustCompile(`NOTA:\s*(\d+)`),
		Copy:        true,
	})
	if err != nil {
		t.Fatalf("Organize: %v", err)
	}
	if len(result.Unclassified) != 1 {
		t.Fatalf("esperava 1 não-classificado, obteve %d", len(result.Unclassified))
	}
	u := result.Unclassified[0]
	if u.Unmatched == nil || !strings.Contains(u.Unmatched.Pattern, "999") {
		t.Fatalf("motivo deveria citar a chave 999 encontrada, obteve: %+v", u.Unmatched)
	}
}

// TestOrganizeCSVModeKeyRegexNoMatchIsUnclassified confere a outra forma de
// não-classificação: o regex de chave simplesmente não casa com o texto.
func TestOrganizeCSVModeKeyRegexNoMatchIsUnclassified(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(inputDir, "doc.pdf"), buildTestPDF(t, []string{"documento sem nota nenhuma"}), 0o644); err != nil {
		t.Fatalf("escrever pdf: %v", err)
	}

	csvMap := CSVMap{Rows: map[string][]string{"001": {"Acme"}}}

	result, err := Organize(context.Background(), OrganizeOptions{
		InputDir:    inputDir,
		OutputDir:   outputDir,
		CSV:         &csvMap,
		CSVKeyRegex: regexp.MustCompile(`NOTA:\s*(\d+)`),
		Copy:        true,
	})
	if err != nil {
		t.Fatalf("Organize: %v", err)
	}
	if len(result.Unclassified) != 1 {
		t.Fatalf("esperava 1 não-classificado, obteve %d", len(result.Unclassified))
	}
	u := result.Unclassified[0]
	if u.Unmatched == nil || u.Unmatched.Level != "chave" {
		t.Fatalf("Unmatched = %+v, esperava Level \"chave\"", u.Unmatched)
	}
}

// TestOrganizeCSVModeDryRunMatchesRealRunOnCollision é o requisito herdado,
// não negociável: simulação e execução real precisam produzir exatamente o
// mesmo resultado também no modo planilha, inclusive com colisão — duas
// chaves diferentes que resolvem para o mesmo destino na planilha.
func TestOrganizeCSVModeDryRunMatchesRealRunOnCollision(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(inputDir, "a-doc.pdf"), buildTestPDF(t, []string{"NOTA: 001"}), 0o644); err != nil {
		t.Fatalf("escrever pdf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "b-doc.pdf"), buildTestPDF(t, []string{"NOTA: 002"}), 0o644); err != nil {
		t.Fatalf("escrever pdf: %v", err)
	}

	// Duas chaves diferentes, mesmo destino na planilha (ex: erro de
	// preenchimento) — força colisão sem depender de nome de arquivo.
	csvMap := CSVMap{
		Rows: map[string][]string{
			"001": {"Acme"},
			"002": {"Acme"},
		},
	}

	opts := func(dryRun bool) OrganizeOptions {
		return OrganizeOptions{
			InputDir:    inputDir,
			OutputDir:   outputDir,
			CSV:         &csvMap,
			CSVKeyRegex: regexp.MustCompile(`NOTA:\s*(\d+)`),
			// Nome de arquivo fixo (ignora a chave) para os dois documentos
			// colidirem no mesmo caminho de destino.
			FilenameRegex: regexp.MustCompile(`(NOTA)`),
			Copy:          true,
			DryRun:        dryRun,
		}
	}

	dryResult, err := Organize(context.Background(), opts(true))
	if err != nil {
		t.Fatalf("Organize (dry-run): %v", err)
	}
	realResult, err := Organize(context.Background(), opts(false))
	if err != nil {
		t.Fatalf("Organize (real): %v", err)
	}

	if len(dryResult.Organized) != len(realResult.Organized) {
		t.Fatalf("Organized diverge: dry=%d real=%d", len(dryResult.Organized), len(realResult.Organized))
	}
	if len(dryResult.Unclassified) != len(realResult.Unclassified) {
		t.Fatalf("Unclassified diverge: dry=%d real=%d", len(dryResult.Unclassified), len(realResult.Unclassified))
	}
	if len(dryResult.Organized) != 1 || len(dryResult.Unclassified) != 1 {
		t.Fatalf("esperava 1 organizado e 1 colidido em ambos os modos; dry: organized=%d unclassified=%d",
			len(dryResult.Organized), len(dryResult.Unclassified))
	}

	dryReport := BuildReport(dryResult)
	realReport := BuildReport(realResult)
	if len(dryReport) != len(realReport) {
		t.Fatalf("relatório diverge em tamanho entre dry-run e real")
	}
	for i := range dryReport {
		if dryReport[i].Destino != realReport[i].Destino || dryReport[i].Classificado != realReport[i].Classificado {
			t.Errorf("linha %d do relatório diverge: dry=%+v real=%+v", i, dryReport[i], realReport[i])
		}
	}
}

// equalStrings compara a e b elemento a elemento, na mesma ordem — usada
// para Levels e para os componentes devolvidos por Lookup, onde a ORDEM
// importa (é a ordem da hierarquia de pastas).
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
