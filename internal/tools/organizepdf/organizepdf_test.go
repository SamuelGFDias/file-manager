package organizepdf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SamuelGFDias/file-manager/internal/history"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func TestParseLevelFlagsBasic(t *testing.T) {
	specs, err := ParseLevelFlags([]string{`fornecedor=FORNECEDOR:\s*(\w+)`})
	if err != nil {
		t.Fatalf("ParseLevelFlags() erro inesperado: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("ParseLevelFlags() devolveu %d specs, esperava 1", len(specs))
	}
	if specs[0].Label != "fornecedor" {
		t.Errorf("specs[0].Label = %q, want %q", specs[0].Label, "fornecedor")
	}
	if specs[0].Regex != `FORNECEDOR:\s*(\w+)` {
		t.Errorf("specs[0].Regex = %q, want %q", specs[0].Regex, `FORNECEDOR:\s*(\w+)`)
	}
}

// TestParseLevelFlagsRegexContainingEquals é o teste crítico: a regex do
// lado direito do "=" quase sempre contém "=" (ex: em asserções como
// "TOTAL\s*=\s*(\d+)"); dividir em TODAS as ocorrências de "=" quebraria a
// regex ao meio. ParseLevelFlags deve dividir só no primeiro "=".
func TestParseLevelFlagsRegexContainingEquals(t *testing.T) {
	specs, err := ParseLevelFlags([]string{`total=TOTAL\s*=\s*(\d+)`})
	if err != nil {
		t.Fatalf("ParseLevelFlags() erro inesperado: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("ParseLevelFlags() devolveu %d specs, esperava 1", len(specs))
	}
	if specs[0].Label != "total" {
		t.Errorf("specs[0].Label = %q, want %q", specs[0].Label, "total")
	}
	if specs[0].Regex != `TOTAL\s*=\s*(\d+)` {
		t.Errorf("specs[0].Regex = %q, want %q (regex inteira preservada, dividida só no primeiro '=')", specs[0].Regex, `TOTAL\s*=\s*(\d+)`)
	}
}

func TestParseLevelFlagsEmptyLabelErrors(t *testing.T) {
	if _, err := ParseLevelFlags([]string{"=abc"}); err == nil {
		t.Fatal("ParseLevelFlags() com rótulo vazio deveria devolver erro")
	}
}

func TestParseLevelFlagsEmptyRegexErrors(t *testing.T) {
	if _, err := ParseLevelFlags([]string{"abc="}); err == nil {
		t.Fatal("ParseLevelFlags() com regex vazia deveria devolver erro")
	}
}

func TestParseLevelFlagsMissingEqualsErrors(t *testing.T) {
	if _, err := ParseLevelFlags([]string{"semigual"}); err == nil {
		t.Fatal("ParseLevelFlags() sem '=' deveria devolver erro")
	}
}

func TestParseLevelFlagsNilInputReturnsEmptySlice(t *testing.T) {
	specs, err := ParseLevelFlags(nil)
	if err != nil {
		t.Fatalf("ParseLevelFlags(nil) erro inesperado: %v", err)
	}
	if len(specs) != 0 {
		t.Fatalf("ParseLevelFlags(nil) devolveu %d specs, esperava 0 (modo somente renomear)", len(specs))
	}
}

func TestBuildLevelsValidSpecsCompile(t *testing.T) {
	levels, err := BuildLevels([]LevelSpec{
		{Label: "fornecedor", Regex: `FORNECEDOR:\s*(\w+)`},
		{Label: "filial", Regex: `FILIAL\s*(\d+)`},
	})
	if err != nil {
		t.Fatalf("BuildLevels() erro inesperado: %v", err)
	}
	if len(levels) != 2 {
		t.Fatalf("BuildLevels() devolveu %d levels, esperava 2", len(levels))
	}
	if levels[0].Label != "fornecedor" || levels[1].Label != "filial" {
		t.Errorf("BuildLevels() não preservou a ordem/rótulos: %+v", levels)
	}
}

func TestBuildLevelsInvalidRegexMentionsLabel(t *testing.T) {
	_, err := BuildLevels([]LevelSpec{
		{Label: "fornecedor", Regex: `FORNECEDOR:\s*(\w+)`},
		{Label: "filial-quebrada", Regex: "["},
	})
	if err == nil {
		t.Fatal("BuildLevels() com regex inválida deveria devolver erro")
	}
	if !strings.Contains(err.Error(), "filial-quebrada") {
		t.Errorf("erro %q deveria mencionar o rótulo do nível problemático (%q)", err.Error(), "filial-quebrada")
	}
}

func TestBuildLevelsEmptySliceReturnsEmptySlice(t *testing.T) {
	levels, err := BuildLevels(nil)
	if err != nil {
		t.Fatalf("BuildLevels(nil) erro inesperado: %v", err)
	}
	if len(levels) != 0 {
		t.Fatalf("BuildLevels(nil) devolveu %d levels, esperava 0", len(levels))
	}
}

func TestMetaID(t *testing.T) {
	got := New().Meta().ID
	want := "organize-pdf"
	if got != want {
		t.Fatalf("Meta().ID = %q, want %q", got, want)
	}
}

func TestCommandUse(t *testing.T) {
	got := New().Command().Use
	want := "organize-pdf"
	if got != want {
		t.Fatalf("Command().Use = %q, want %q", got, want)
	}
}

func TestDocFlagsMatchesParams(t *testing.T) {
	tl := New()

	doc := tl.Doc()
	params := tl.params()
	cmd := tl.Command()

	if len(doc.Flags) != len(params) {
		t.Fatalf("Doc().Flags tem %d entradas, esperava %d (mesmo tamanho de params())", len(doc.Flags), len(params))
	}

	for _, fd := range doc.Flags {
		if cmd.Flags().Lookup(fd.Name) == nil {
			t.Errorf("Doc().Flags contém %q, mas Command().Flags() não tem essa flag", fd.Name)
		}
	}
}

func TestExpectedFlagsExist(t *testing.T) {
	cmd := New().Command()

	cases := []struct {
		name      string
		shorthand string
	}{
		{"input", "i"},
		{"output", "o"},
		{"level", ""},
		{"filename-regex", ""},
		{"move", ""},
		{"unclassified-dir", ""},
		{"overwrite", ""},
		{"dry-run", ""},
		{"sample", ""},
		{"ocr", ""},
		{"ocr-lang", ""},
		{"report", ""},
		{"report-format", ""},
		{"csv", ""},
		{"csv-key-regex", ""},
		{"csv-key-column", ""},
		{"csv-levels", ""},
	}

	if len(cases) != 17 {
		t.Fatalf("teste declara %d flags, esperava 17", len(cases))
	}

	for _, c := range cases {
		f := cmd.Flags().Lookup(c.name)
		if f == nil {
			t.Fatalf("flag %q não encontrada", c.name)
		}
		if f.Shorthand != c.shorthand {
			t.Errorf("flag %q: shorthand = %q, want %q", c.name, f.Shorthand, c.shorthand)
		}
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()

	if opts.UnclassifiedDir != "sem-classificacao" {
		t.Errorf("defaultOptions().UnclassifiedDir = %q, want %q", opts.UnclassifiedDir, "sem-classificacao")
	}
	if opts.Move != false {
		t.Error("defaultOptions().Move deveria ser false — o padrão é copiar (não destrutivo); se isso falhou, a ferramenta ficou destrutiva por padrão")
	}
	if opts.OCR != "auto" {
		t.Errorf("defaultOptions().OCR = %q, want %q", opts.OCR, "auto")
	}
	if opts.OCRLang != "por" {
		t.Errorf("defaultOptions().OCRLang = %q, want %q", opts.OCRLang, "por")
	}
	if opts.ReportFormat != "csv" {
		t.Errorf("defaultOptions().ReportFormat = %q, want %q", opts.ReportFormat, "csv")
	}
	if opts.Report != "" {
		t.Errorf("defaultOptions().Report = %q, want vazio (relatório desligado por padrão)", opts.Report)
	}
}

func TestRunEmptyOptionsErrorsWithoutPanic(t *testing.T) {
	tl := New()
	tl.opts = Options{}

	if _, err := tl.run(); err == nil {
		t.Fatal("run() com Options vazias deveria devolver erro (input/output faltando)")
	}
}

func TestRunInvalidFilenameRegexErrorsWithoutPanic(t *testing.T) {
	tl := New()
	tl.opts = Options{
		InputDir:      "entrada",
		OutputDir:     "saida",
		FilenameRegex: "[",
	}

	_, err := tl.run()
	if err == nil {
		t.Fatal("run() com FilenameRegex inválida (\"[\") deveria devolver erro de compilação")
	}
}

func TestRunInvalidLevelRegexErrorsWithoutPanic(t *testing.T) {
	tl := New()
	tl.opts = Options{
		InputDir:  "entrada",
		OutputDir: "saida",
		Levels:    []LevelSpec{{Label: "fornecedor", Regex: "["}},
	}

	_, err := tl.run()
	if err == nil {
		t.Fatal("run() com regex de nível inválida deveria devolver erro")
	}
	if !strings.Contains(err.Error(), "fornecedor") {
		t.Errorf("erro %q deveria mencionar o rótulo do nível problemático", err.Error())
	}
}

func TestRunInvalidOCRModeErrorsWithoutPanic(t *testing.T) {
	tl := New()
	tl.opts = Options{
		InputDir:  "entrada",
		OutputDir: "saida",
		OCR:       "xyz",
	}

	_, err := tl.run()
	if err == nil {
		t.Fatal("run() com --ocr inválido (\"xyz\") deveria devolver erro")
	}
	for _, want := range []string{"auto", "always", "never"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("erro %q deveria mencionar o valor válido %q", err.Error(), want)
		}
	}
}

func TestTextOptionsOCRNeverHasNoEngine(t *testing.T) {
	tl := New()
	tl.opts.OCR = "never"
	tl.opts.OCRLang = "por"

	opts, err := tl.textOptions()
	if err != nil {
		t.Fatalf("textOptions() erro inesperado: %v", err)
	}
	if opts.Engine != nil {
		t.Errorf("textOptions() com OCR=never deveria devolver Engine nil, obteve %#v", opts.Engine)
	}
}

func TestTextOptionsOCRAutoHasEngine(t *testing.T) {
	tl := New()
	tl.opts.OCR = "auto"
	tl.opts.OCRLang = "por"

	opts, err := tl.textOptions()
	if err != nil {
		t.Fatalf("textOptions() erro inesperado: %v", err)
	}
	if opts.Engine == nil {
		t.Error("textOptions() com OCR=auto deveria devolver Engine != nil (mesmo que o motor não esteja instalado — Engine só reflete se o modo permite OCR, não se o Tesseract está presente)")
	}
}

func TestProfileNotNil(t *testing.T) {
	tl := New()
	if tl.Profile() == nil {
		t.Fatal("Profile() não deveria ser nil")
	}
}

func TestProfileApplyWrongTypeErrorsWithoutPanic(t *testing.T) {
	tl := New()
	p := tl.Profile()

	_, err := p.Apply("not-an-options-pointer")
	if err == nil {
		t.Fatal("Apply() com tipo errado deveria devolver erro, não panic")
	}
}

func TestProfileEmpty(t *testing.T) {
	tl := New()
	p := tl.Profile()

	empty, ok := p.Empty().(*Options)
	if !ok {
		t.Fatalf("Empty() devolveu tipo %T, esperava *Options", p.Empty())
	}
	if empty.UnclassifiedDir != "sem-classificacao" {
		t.Errorf("Empty().UnclassifiedDir = %q, want %q", empty.UnclassifiedDir, "sem-classificacao")
	}
	if empty.Move != false {
		t.Error("Empty().Move deveria ser false")
	}
}

// TestCountPDFsWithFilesAndSubdir é o caso do bug relatado: uma pasta com
// PDFs, um arquivo não-PDF e uma subpasta deve contar só os PDFs.
func TestCountPDFsWithFilesAndSubdir(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "a.pdf"), []byte("pdf"), 0o644); err != nil {
		t.Fatalf("erro ao criar a.pdf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.pdf"), []byte("pdf"), 0o644); err != nil {
		t.Fatalf("erro ao criar b.pdf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notas.txt"), []byte("txt"), 0o644); err != nil {
		t.Fatalf("erro ao criar notas.txt: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subpasta"), 0o755); err != nil {
		t.Fatalf("erro ao criar subpasta: %v", err)
	}

	count, err := countPDFs(dir)
	if err != nil {
		t.Fatalf("countPDFs() erro inesperado: %v", err)
	}
	if count != 2 {
		t.Errorf("countPDFs() = %d, want 2", count)
	}
}

// TestCountPDFsEmptyDir reproduz exatamente o bug relatado: pasta de origem
// sem nenhum PDF (ex: o usuário escolheu por engano o diretório do próprio
// executável) deve contar 0, sem erro — é isso que a correção 2 usa para
// barrar o fluxo antes da calibração.
func TestCountPDFsEmptyDir(t *testing.T) {
	dir := t.TempDir()

	count, err := countPDFs(dir)
	if err != nil {
		t.Fatalf("countPDFs() erro inesperado: %v", err)
	}
	if count != 0 {
		t.Errorf("countPDFs() = %d, want 0", count)
	}
}

func TestCountPDFsNonexistentDirErrors(t *testing.T) {
	_, err := countPDFs(filepath.Join(t.TempDir(), "nao-existe"))
	if err == nil {
		t.Fatal("countPDFs() com diretório inexistente deveria devolver erro")
	}
}

// TestRunWritesHistoryManifestOnRealMove prova a ponta a ponta da injeção
// de history.Recorder através de organizeOpts (ver historyRecorder em
// command.go): uma execução real com --move grava um manifesto que pode
// ser lido de volta via internal/history. t.Setenv isola XDG_CONFIG_HOME
// num diretório temporário, exatamente como o harness e2e faz — nunca toca
// no diretório de configuração real da máquina de quem roda os testes.
//
// O conteúdo do "PDF" de teste não precisa ser um PDF de verdade: mesmo
// falhando a extração de texto, o arquivo cai em sem-classificacao, e essa
// movimentação também deve ser registrada (ver contrato de
// OrganizeOptions.Recorder e OrganizeResult).
func TestRunWritesHistoryManifestOnRealMove(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	inputDir := t.TempDir()
	outputDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(inputDir, "doc.pdf"), []byte("conteudo qualquer"), 0o644); err != nil {
		t.Fatalf("criar pdf de teste: %v", err)
	}

	tl := New()
	tl.opts = Options{
		InputDir:        inputDir,
		OutputDir:       outputDir,
		Move:            true,
		UnclassifiedDir: "sem-classificacao",
		OCR:             "never",
	}

	if _, err := tl.run(); err != nil {
		t.Fatalf("run() erro inesperado: %v", err)
	}

	headers, _, err := history.List()
	if err != nil {
		t.Fatalf("history.List() erro inesperado: %v", err)
	}
	if len(headers) != 1 {
		t.Fatalf("esperava 1 manifesto gravado, obteve %d", len(headers))
	}

	m := headers[0]
	if m.Tool != "organize-pdf" {
		t.Errorf("Tool = %q, esperava %q", m.Tool, "organize-pdf")
	}
	if m.Action != history.ActionMove {
		t.Errorf("Action = %q, esperava %q", m.Action, history.ActionMove)
	}
	if m.EntryCount != 1 {
		t.Fatalf("esperava 1 entrada no manifesto, obteve %d", m.EntryCount)
	}
	if !filepath.IsAbs(m.InputDir) || !filepath.IsAbs(m.OutputDir) {
		t.Errorf("InputDir/OutputDir do manifesto deveriam ser absolutos: %q / %q", m.InputDir, m.OutputDir)
	}

	// Load() continua devolvendo o Manifest completo, com Entries — é o
	// que o desfazer usa; List() (acima) só devolve o cabeçalho.
	full, err := history.Load(m.ID)
	if err != nil {
		t.Fatalf("history.Load(%q) erro inesperado: %v", m.ID, err)
	}
	if len(full.Entries) != 1 {
		t.Fatalf("esperava 1 entrada em Load(), obteve %d: %+v", len(full.Entries), full.Entries)
	}
}

// TestRunDryRunDoesNotWriteHistoryManifest é a garantia pedida
// explicitamente: uma simulação não pode gerar histórico. Confirma que o
// diretório de histórico continua vazio (ou nem chega a ser criado) depois
// de um run() com DryRun: true.
func TestRunDryRunDoesNotWriteHistoryManifest(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	inputDir := t.TempDir()
	outputDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(inputDir, "doc.pdf"), []byte("conteudo qualquer"), 0o644); err != nil {
		t.Fatalf("criar pdf de teste: %v", err)
	}

	tl := New()
	tl.opts = Options{
		InputDir:        inputDir,
		OutputDir:       outputDir,
		UnclassifiedDir: "sem-classificacao",
		OCR:             "never",
		DryRun:          true,
	}

	if _, err := tl.run(); err != nil {
		t.Fatalf("run() erro inesperado: %v", err)
	}

	headers, _, err := history.List()
	if err != nil {
		t.Fatalf("history.List() erro inesperado: %v", err)
	}
	if len(headers) != 0 {
		t.Fatalf("esperava 0 manifestos após DryRun, obteve %d: %+v", len(headers), headers)
	}
}

// TestRunReportsPrunedPendingManifestsInDetails prova a ponta a ponta do
// aviso pedido: quando a poda automática disparada por history.Save (ver
// historyRecorder) remove um manifesto PENDENTE (nunca desfeito) — não
// apenas um já desfeito —, isso precisa aparecer em Result.Details. Apagar
// em silêncio a capacidade de desfazer uma operação seria exatamente a
// surpresa que este projeto existe para evitar.
//
// O manifesto antigo é criado via history.Save (com CreatedAt recente, para
// não ser podado na hora) e depois tem created_at reescrito DIRETAMENTE no
// arquivo, contornando uma segunda chamada a Save — que já dispararia a
// poda automática e removeria o próprio manifesto no ato de criá-lo "velho
// o bastante". Só a poda disparada pela gravação REAL feita pelo run() de
// organize-pdf, abaixo, deve alcançá-lo.
func TestRunReportsPrunedPendingManifestsInDetails(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	path, _, err := history.Save(history.Manifest{
		Tool:      "organize-pdf",
		InputDir:  "/tmp/origem-antiga",
		OutputDir: "/tmp/destino-antigo",
		Action:    history.ActionCopy,
	})
	if err != nil {
		t.Fatalf("history.Save(manifesto antigo): %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler manifesto antigo: %v", err)
	}
	var old history.Manifest
	if err := yaml.Unmarshal(raw, &old); err != nil {
		t.Fatalf("decodificar manifesto antigo: %v", err)
	}
	old.CreatedAt = time.Now().Add(-(history.PrunePendingAfter + 24*time.Hour))
	rewritten, err := yaml.Marshal(&old)
	if err != nil {
		t.Fatalf("recodificar manifesto antigo: %v", err)
	}
	if err := os.WriteFile(path, rewritten, 0o644); err != nil {
		t.Fatalf("regravar manifesto antigo com created_at antigo: %v", err)
	}

	inputDir := t.TempDir()
	outputDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(inputDir, "doc.pdf"), []byte("conteudo qualquer"), 0o644); err != nil {
		t.Fatalf("criar pdf de teste: %v", err)
	}

	tl := New()
	tl.opts = Options{
		InputDir:        inputDir,
		OutputDir:       outputDir,
		Move:            true,
		UnclassifiedDir: "sem-classificacao",
		OCR:             "never",
	}

	result, err := tl.run()
	if err != nil {
		t.Fatalf("run() erro inesperado: %v", err)
	}

	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "removido") && strings.Contains(d, "desfeito") {
			found = true
		}
	}
	if !found {
		t.Fatalf("esperava um detalhe avisando sobre o manifesto pendente removido pela poda automática; Details = %v", result.Details)
	}

	if _, err := history.Load(old.ID); err == nil {
		t.Fatalf("manifesto pendente antigo (%q) deveria ter sido removido pela poda automática", old.ID)
	}
}

// TestSampleOutsideInputSampleInside cobre o caso normal: amostra dentro da
// pasta de origem, sem aviso.
func TestSampleOutsideInputSampleInside(t *testing.T) {
	dir := t.TempDir()
	samplePath := filepath.Join(dir, "amostra.pdf")
	if err := os.WriteFile(samplePath, []byte("pdf"), 0o644); err != nil {
		t.Fatalf("erro ao criar amostra.pdf: %v", err)
	}

	outside, err := sampleOutsideInput(samplePath, dir)
	if err != nil {
		t.Fatalf("sampleOutsideInput() erro inesperado: %v", err)
	}
	if outside {
		t.Error("sampleOutsideInput() = true, want false (amostra está dentro da pasta de origem)")
	}
}

// TestSampleOutsideInputSampleOutside reproduz o cenário relatado pelo
// usuário: amostra escolhida numa pasta diferente da origem (ex: ~/Downloads
// enquanto a origem era ~/.file_manager).
func TestSampleOutsideInputSampleOutside(t *testing.T) {
	inputDir := t.TempDir()
	otherDir := t.TempDir()
	samplePath := filepath.Join(otherDir, "amostra.pdf")
	if err := os.WriteFile(samplePath, []byte("pdf"), 0o644); err != nil {
		t.Fatalf("erro ao criar amostra.pdf: %v", err)
	}

	outside, err := sampleOutsideInput(samplePath, inputDir)
	if err != nil {
		t.Fatalf("sampleOutsideInput() erro inesperado: %v", err)
	}
	if !outside {
		t.Error("sampleOutsideInput() = false, want true (amostra está fora da pasta de origem)")
	}
}

// TestSampleOutsideInputMixedRelativeAndAbsolutePaths é o caso que uma
// comparação ingênua de strings erraria: caminhos relativo e absoluto
// apontando para o mesmo diretório devem ser reconhecidos como iguais.
func TestSampleOutsideInputMixedRelativeAndAbsolutePaths(t *testing.T) {
	dir := t.TempDir()
	samplePath := filepath.Join(dir, "amostra.pdf")
	if err := os.WriteFile(samplePath, []byte("pdf"), 0o644); err != nil {
		t.Fatalf("erro ao criar amostra.pdf: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("erro ao obter diretório de trabalho: %v", err)
	}
	rel, err := filepath.Rel(wd, dir)
	if err != nil {
		t.Fatalf("erro ao calcular caminho relativo: %v", err)
	}

	outside, err := sampleOutsideInput(samplePath, rel)
	if err != nil {
		t.Fatalf("sampleOutsideInput() erro inesperado: %v", err)
	}
	if outside {
		t.Error("sampleOutsideInput() = true, want false (mesmo diretório, só que um dos lados é relativo)")
	}
}

func TestNormalizeReportFormatValid(t *testing.T) {
	for _, raw := range []string{"csv", "CSV", "  csv  ", "json", "JSON", ""} {
		got, err := NormalizeReportFormat(raw)
		if err != nil {
			t.Fatalf("NormalizeReportFormat(%q) erro inesperado: %v", raw, err)
		}
		if got != "csv" && got != "json" {
			t.Errorf("NormalizeReportFormat(%q) = %q, esperava \"csv\" ou \"json\"", raw, got)
		}
	}

	got, err := NormalizeReportFormat("")
	if err != nil || got != "csv" {
		t.Errorf("NormalizeReportFormat(\"\") = (%q, %v), esperava (\"csv\", nil) — vazio deveria cair no default", got, err)
	}
}

func TestNormalizeReportFormatInvalid(t *testing.T) {
	_, err := NormalizeReportFormat("xml")
	if err == nil {
		t.Fatal("NormalizeReportFormat(\"xml\") deveria devolver erro")
	}
	for _, want := range []string{"csv", "json"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("erro %q deveria mencionar o formato válido %q", err.Error(), want)
		}
	}
}

// TestRunInvalidReportFormatErrorsBeforeProcessing prova a exigência
// central de --report-format: um erro de digitação na flag precisa ser
// detectado ANTES de qualquer arquivo ser tocado. Usa uma pasta de entrada
// com um PDF de verdade (mesmo que mínimo) para que, se a validação
// acontecesse tarde demais, o teste pegasse o arquivo sendo processado
// mesmo assim.
func TestRunInvalidReportFormatErrorsBeforeProcessing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	inputDir := t.TempDir()
	outputDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(inputDir, "doc.pdf"), []byte("conteudo qualquer"), 0o644); err != nil {
		t.Fatalf("criar pdf de teste: %v", err)
	}

	tl := New()
	tl.opts = Options{
		InputDir:     inputDir,
		OutputDir:    outputDir,
		OCR:          "never",
		Report:       filepath.Join(t.TempDir(), "relatorio.csv"),
		ReportFormat: "xml",
	}

	if _, err := tl.run(); err == nil {
		t.Fatal("run() com --report-format inválido deveria devolver erro")
	}

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatalf("ler outputDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("outputDir tem %d entradas; --report-format inválido deveria falhar ANTES de processar qualquer arquivo", len(entries))
	}
}

// TestRunGeneratesReportOnDryRun prova a exigência explícita: o relatório é
// gerado mesmo em simulação, já que é justamente aí que ele mais serve
// (conferir a classificação antes de aplicar de verdade).
func TestRunGeneratesReportOnDryRun(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	inputDir := t.TempDir()
	outputDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(inputDir, "doc.pdf"), []byte("conteudo qualquer"), 0o644); err != nil {
		t.Fatalf("criar pdf de teste: %v", err)
	}

	reportPath := filepath.Join(t.TempDir(), "relatorio.csv")

	tl := New()
	tl.opts = Options{
		InputDir:        inputDir,
		OutputDir:       outputDir,
		UnclassifiedDir: "sem-classificacao",
		OCR:             "never",
		DryRun:          true,
		Report:          reportPath,
		ReportFormat:    "csv",
	}

	result, err := tl.run()
	if err != nil {
		t.Fatalf("run() erro inesperado: %v", err)
	}

	if _, statErr := os.Stat(reportPath); statErr != nil {
		t.Fatalf("relatório não foi criado em DryRun: %v", statErr)
	}

	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "relatório gravado em") {
			found = true
		}
	}
	if !found {
		t.Errorf("Result.Details não confirma o caminho do relatório gerado: %+v", result.Details)
	}

	// Em DryRun, nada é copiado/movido: o outputDir deve continuar vazio
	// fora do arquivo de relatório, que fica em outro diretório.
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatalf("ler outputDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("outputDir tem %d entradas após DryRun, esperava 0", len(entries))
	}
}

// TestRunReportWriteFailureDoesNotFailOperation prova a regra explícita:
// falha ao gravar o relatório (aqui, path aponta para um diretório já
// existente, então os.Create falha) não faz run() devolver erro — a
// organização já aconteceu e não pode ser desfeita por causa de um
// artefato acessório. O aviso deve aparecer em Result.Details.
func TestRunReportWriteFailureDoesNotFailOperation(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	inputDir := t.TempDir()
	outputDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(inputDir, "doc.pdf"), []byte("conteudo qualquer"), 0o644); err != nil {
		t.Fatalf("criar pdf de teste: %v", err)
	}

	// reportPath é um DIRETÓRIO existente, não um arquivo: os.Create deve
	// falhar.
	reportPath := t.TempDir()

	tl := New()
	tl.opts = Options{
		InputDir:        inputDir,
		OutputDir:       outputDir,
		UnclassifiedDir: "sem-classificacao",
		OCR:             "never",
		Report:          reportPath,
		ReportFormat:    "csv",
	}

	result, err := tl.run()
	if err != nil {
		t.Fatalf("run() não deveria devolver erro por falha ao gravar o relatório, obteve: %v", err)
	}

	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "não foi possível gravar o relatório") {
			found = true
		}
	}
	if !found {
		t.Errorf("Result.Details deveria conter um aviso sobre a falha ao gravar o relatório: %+v", result.Details)
	}

	// A organização em si deve ter acontecido normalmente: o arquivo saiu
	// de inputDir (Move não foi pedido, então ele continua lá, mas uma
	// cópia deve existir em outputDir/sem-classificacao, já que "conteudo
	// qualquer" não é um PDF de verdade e não vai casar com nada).
	unclassifiedEntries, err := os.ReadDir(filepath.Join(outputDir, "sem-classificacao"))
	if err != nil {
		t.Fatalf("ler outputDir/sem-classificacao: %v", err)
	}
	if len(unclassifiedEntries) != 1 {
		t.Fatalf("esperava 1 arquivo em sem-classificacao, obteve %d — a organização deveria ter acontecido normalmente apesar da falha do relatório", len(unclassifiedEntries))
	}
}

// TestOCRCompletion garante que a completação de --ocr devolve exatamente
// os três valores aceitos, sem completar arquivo.
func TestOCRCompletion(t *testing.T) {
	cmd := New().Command()

	fn, ok := cmd.GetFlagCompletionFunc("ocr")
	if !ok {
		t.Fatal("nenhuma função de completação registrada para --ocr")
	}

	got, directive := fn(cmd, nil, "")

	want := []string{"auto", "always", "never"}
	if len(got) != len(want) {
		t.Fatalf("completação de --ocr = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("completação de --ocr[%d] = %q, want %q", i, got[i], w)
		}
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
}

// TestOCRLangCompletionFallsBackWithoutTesseract prova que, sem o
// tesseract disponível, a completação de --ocr-lang devolve a lista fixa
// conhecida (por, eng) em vez de travar ou falhar.
func TestOCRLangCompletionFallsBackWithoutTesseract(t *testing.T) {
	t.Setenv("PATH", "/nao-existe/bin")
	t.Setenv("TESSERACT_PATH", "/nao-existe/tesseract")

	cmd := New().Command()

	fn, ok := cmd.GetFlagCompletionFunc("ocr-lang")
	if !ok {
		t.Fatal("nenhuma função de completação registrada para --ocr-lang")
	}

	got, directive := fn(cmd, nil, "")

	if len(got) != 2 {
		t.Fatalf("completação de --ocr-lang sem tesseract = %v, esperava 2 entradas (por, eng)", got)
	}
	if !strings.HasPrefix(got[0], "por\t") || !strings.HasPrefix(got[1], "eng\t") {
		t.Errorf("completação de --ocr-lang sem tesseract = %v, esperava prefixos \"por\\t\" e \"eng\\t\"", got)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
}

// TestReportFormatCompletion garante que a completação de --report-format
// devolve exatamente os dois formatos aceitos, sem completar arquivo.
func TestReportFormatCompletion(t *testing.T) {
	cmd := New().Command()

	fn, ok := cmd.GetFlagCompletionFunc("report-format")
	if !ok {
		t.Fatal("nenhuma função de completação registrada para --report-format")
	}

	got, directive := fn(cmd, nil, "")

	want := []string{"csv", "json"}
	if len(got) != len(want) {
		t.Fatalf("completação de --report-format = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("completação de --report-format[%d] = %q, want %q", i, got[i], w)
		}
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
}

// --- Validação de --csv e flags relacionadas --------------------------------

func TestValidateCSVOptionsCSVWithLevelIsError(t *testing.T) {
	err := ValidateCSVOptions("./planilha.csv", `NOTA:\s*(\d+)`, "", nil, true)
	if err == nil {
		t.Fatal("--csv junto com --level deveria devolver erro")
	}
	if !strings.Contains(err.Error(), "--csv") || !strings.Contains(err.Error(), "--level") {
		t.Errorf("erro %q deveria mencionar --csv e --level", err.Error())
	}
}

func TestValidateCSVOptionsCSVWithoutKeyRegexIsError(t *testing.T) {
	err := ValidateCSVOptions("./planilha.csv", "", "", nil, false)
	if err == nil {
		t.Fatal("--csv sem --csv-key-regex deveria devolver erro")
	}
	if !strings.Contains(err.Error(), "--csv-key-regex") {
		t.Errorf("erro %q deveria mencionar --csv-key-regex", err.Error())
	}
}

func TestValidateCSVOptionsExtraFlagsWithoutCSVAreError(t *testing.T) {
	cases := []struct {
		name         string
		keyRegex     string
		keyColumn    string
		levelColumns []string
	}{
		{"csv-key-regex sozinho", `NOTA:\s*(\d+)`, "", nil},
		{"csv-key-column sozinho", "", "NOTA", nil},
		{"csv-levels sozinho", "", "", []string{"CIDADE"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateCSVOptions("", c.keyRegex, c.keyColumn, c.levelColumns, false)
			if err == nil {
				t.Fatalf("%s sem --csv deveria devolver erro", c.name)
			}
			if !strings.Contains(err.Error(), "--csv") {
				t.Errorf("erro %q deveria mencionar --csv", err.Error())
			}
		})
	}
}

func TestValidateCSVOptionsValidCombinationsOK(t *testing.T) {
	if err := ValidateCSVOptions("", "", "", nil, false); err != nil {
		t.Errorf("sem nenhuma flag de csv nem --level não deveria dar erro: %v", err)
	}
	if err := ValidateCSVOptions("", "", "", nil, true); err != nil {
		t.Errorf("só --level (sem --csv) não deveria dar erro: %v", err)
	}
	if err := ValidateCSVOptions("./planilha.csv", `NOTA:\s*(\d+)`, "", nil, false); err != nil {
		t.Errorf("--csv com --csv-key-regex, sem --level, não deveria dar erro: %v", err)
	}
	if err := ValidateCSVOptions("./planilha.csv", `NOTA:\s*(\d+)`, "NOTA", []string{"CIDADE", "BAIRRO"}, false); err != nil {
		t.Errorf("--csv com todas as flags relacionadas preenchidas não deveria dar erro: %v", err)
	}
}

// --- run() com --csv ---------------------------------------------------------

func TestRunCSVWithLevelErrorsWithoutPanic(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	inputDir := t.TempDir()
	outputDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(inputDir, "doc.pdf"), []byte("conteudo qualquer"), 0o644); err != nil {
		t.Fatalf("criar pdf de teste: %v", err)
	}

	tl := New()
	tl.opts = Options{
		InputDir:    inputDir,
		OutputDir:   outputDir,
		OCR:         "never",
		CSV:         "./planilha.csv",
		CSVKeyRegex: `NOTA:\s*(\d+)`,
		Levels:      []LevelSpec{{Label: "fornecedor", Regex: `FORNECEDOR:\s*(\w+)`}},
	}

	if _, err := tl.run(); err == nil {
		t.Fatal("run() com --csv e --level deveria devolver erro")
	}

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatalf("ler outputDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("outputDir tem %d entradas; a validação de --csv+--level deveria falhar ANTES de processar qualquer arquivo", len(entries))
	}
}

func TestRunCSVWithoutKeyRegexErrorsWithoutPanic(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	inputDir := t.TempDir()
	outputDir := t.TempDir()

	tl := New()
	tl.opts = Options{
		InputDir:  inputDir,
		OutputDir: outputDir,
		OCR:       "never",
		CSV:       "./planilha.csv",
	}

	if _, err := tl.run(); err == nil {
		t.Fatal("run() com --csv sem --csv-key-regex deveria devolver erro")
	}
}

func TestRunCSVFlagsWithoutCSVErrorsWithoutPanic(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	inputDir := t.TempDir()
	outputDir := t.TempDir()

	tl := New()
	tl.opts = Options{
		InputDir:     inputDir,
		OutputDir:    outputDir,
		OCR:          "never",
		CSVKeyColumn: "NOTA",
	}

	if _, err := tl.run(); err == nil {
		t.Fatal("run() com --csv-key-column sem --csv deveria devolver erro")
	}
}

func TestRunCSVNonexistentFileErrorsWithPathInMessage(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	inputDir := t.TempDir()
	outputDir := t.TempDir()
	csvPath := filepath.Join(t.TempDir(), "nao-existe.csv")

	tl := New()
	tl.opts = Options{
		InputDir:    inputDir,
		OutputDir:   outputDir,
		OCR:         "never",
		CSV:         csvPath,
		CSVKeyRegex: `NOTA:\s*(\d+)`,
	}

	_, err := tl.run()
	if err == nil {
		t.Fatal("run() com planilha inexistente deveria devolver erro")
	}
	if !strings.Contains(err.Error(), csvPath) {
		t.Errorf("erro %q deveria citar o caminho da planilha (%q)", err.Error(), csvPath)
	}
}

func TestRunCSVInvalidKeyRegexErrorsWithoutPanic(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	dir := t.TempDir()
	csvPath := filepath.Join(dir, "planilha.csv")
	if err := os.WriteFile(csvPath, []byte("NOTA,CIDADE\n001,Acme\n"), 0o644); err != nil {
		t.Fatalf("criar planilha de teste: %v", err)
	}

	tl := New()
	tl.opts = Options{
		InputDir:    t.TempDir(),
		OutputDir:   t.TempDir(),
		OCR:         "never",
		CSV:         csvPath,
		CSVKeyRegex: "[",
	}

	if _, err := tl.run(); err == nil {
		t.Fatal("run() com --csv-key-regex inválida deveria devolver erro")
	}
}

// TestRunCSVWiresLoadedMapIntoOrganize confere que run() carrega a planilha
// e passa CSV/CSVKeyRegex adiante para pdfutil.Organize: o "PDF" de teste
// não é um PDF de verdade (a extração de texto falha, então ele cai em
// sem-classificacao, motivo "texto" — não chega a testar a resolução via
// CSVMap.Lookup, já coberta em internal/pdfutil), mas confirma que o
// caminho todo (carregar a planilha, compilar a regex, montar
// OrganizeOptions, chamar Organize, montar o relatório de Details) roda sem
// erro e sem pânico com --csv de ponta a ponta.
func TestRunCSVWiresLoadedMapIntoOrganize(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	inputDir := t.TempDir()
	outputDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(inputDir, "doc.pdf"), []byte("conteudo qualquer, nao e pdf de verdade"), 0o644); err != nil {
		t.Fatalf("criar pdf de teste: %v", err)
	}

	csvPath := filepath.Join(t.TempDir(), "planilha.csv")
	if err := os.WriteFile(csvPath, []byte("NOTA,CIDADE,BAIRRO\n001,Sao Goncalo,Laranjal\n"), 0o644); err != nil {
		t.Fatalf("criar planilha de teste: %v", err)
	}

	tl := New()
	tl.opts = Options{
		InputDir:        inputDir,
		OutputDir:       outputDir,
		UnclassifiedDir: "sem-classificacao",
		OCR:             "never",
		CSV:             csvPath,
		CSVKeyRegex:     `NOTA:\s*(\d+)`,
	}

	result, err := tl.run()
	if err != nil {
		t.Fatalf("run() com --csv válido não deveria devolver erro: %v", err)
	}

	unclassifiedEntries, err := os.ReadDir(filepath.Join(outputDir, "sem-classificacao"))
	if err != nil {
		t.Fatalf("ler outputDir/sem-classificacao: %v", err)
	}
	if len(unclassifiedEntries) != 1 {
		t.Fatalf("esperava 1 arquivo em sem-classificacao (texto não extraível), obteve %d", len(unclassifiedEntries))
	}

	if !strings.Contains(result.Summary, "0 de 1") {
		t.Errorf("Summary = %q, esperava indicar 0 de 1 organizados", result.Summary)
	}
}

func TestRunCSVEmptyLevelCellWarningReachesDetails(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	inputDir := t.TempDir()
	outputDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(inputDir, "doc.pdf"), []byte("conteudo qualquer"), 0o644); err != nil {
		t.Fatalf("criar pdf de teste: %v", err)
	}

	// Célula de nível ("CIDADE") vazia na única linha da planilha: deve
	// virar um aviso, não um erro, e chegar até Result.Details.
	csvPath := filepath.Join(t.TempDir(), "planilha.csv")
	if err := os.WriteFile(csvPath, []byte("NOTA,CIDADE\n001,\n"), 0o644); err != nil {
		t.Fatalf("criar planilha de teste: %v", err)
	}

	tl := New()
	tl.opts = Options{
		InputDir:        inputDir,
		OutputDir:       outputDir,
		UnclassifiedDir: "sem-classificacao",
		OCR:             "never",
		CSV:             csvPath,
		CSVKeyRegex:     `NOTA:\s*(\d+)`,
	}

	result, err := tl.run()
	if err != nil {
		t.Fatalf("run() não deveria devolver erro: %v", err)
	}

	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "001") && strings.Contains(d, "CIDADE") {
			found = true
		}
	}
	if !found {
		t.Errorf("Result.Details deveria conter o aviso de célula vazia (chave 001, coluna CIDADE): %+v", result.Details)
	}
}

// --- Completação de --csv e --csv-levels ------------------------------------

// TestCSVCompletionFiltersCSVExtension garante que --csv delega a
// completação de arquivo ao cobra, filtrando pela extensão .csv.
func TestCSVCompletionFiltersCSVExtension(t *testing.T) {
	cmd := New().Command()

	fn, ok := cmd.GetFlagCompletionFunc("csv")
	if !ok {
		t.Fatal("nenhuma função de completação registrada para --csv")
	}

	got, directive := fn(cmd, nil, "")
	if directive != cobra.ShellCompDirectiveFilterFileExt {
		t.Errorf("directive = %v, want ShellCompDirectiveFilterFileExt", directive)
	}
	if len(got) != 1 || got[0] != "csv" {
		t.Errorf("completação de --csv = %v, esperava [\"csv\"]", got)
	}
}

// TestCSVLevelsCompletionReadsHeaderFromCSVFlag prova o caso que dá o
// ganho real: com --csv já apontando para uma planilha de verdade,
// --csv-levels oferece os nomes de coluna do cabeçalho.
func TestCSVLevelsCompletionReadsHeaderFromCSVFlag(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "planilha.csv")
	if err := os.WriteFile(csvPath, []byte("NOTA,CIDADE,BAIRRO\n001,Sao Goncalo,Laranjal\n"), 0o644); err != nil {
		t.Fatalf("criar planilha de teste: %v", err)
	}

	cmd := New().Command()
	if err := cmd.Flags().Set("csv", csvPath); err != nil {
		t.Fatalf("cmd.Flags().Set(\"csv\", ...): %v", err)
	}

	fn, ok := cmd.GetFlagCompletionFunc("csv-levels")
	if !ok {
		t.Fatal("nenhuma função de completação registrada para --csv-levels")
	}

	got, directive := fn(cmd, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}

	want := []string{"NOTA", "CIDADE", "BAIRRO"}
	if len(got) != len(want) {
		t.Fatalf("completação de --csv-levels = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("completação de --csv-levels[%d] = %q, want %q", i, got[i], w)
		}
	}
}

// TestCSVLevelsCompletionEmptyWithoutCSVFlag garante que, sem --csv
// preenchido, --csv-levels devolve lista vazia sem erro.
func TestCSVLevelsCompletionEmptyWithoutCSVFlag(t *testing.T) {
	cmd := New().Command()

	fn, ok := cmd.GetFlagCompletionFunc("csv-levels")
	if !ok {
		t.Fatal("nenhuma função de completação registrada para --csv-levels")
	}

	got, directive := fn(cmd, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
	if len(got) != 0 {
		t.Errorf("completação de --csv-levels sem --csv = %v, esperava lista vazia", got)
	}
}

// TestCSVLevelsCompletionEmptyWhenFileMissing garante que, com --csv
// apontando para um arquivo inexistente, --csv-levels devolve lista vazia
// sem erro (nunca propaga o erro de leitura).
func TestCSVLevelsCompletionEmptyWhenFileMissing(t *testing.T) {
	cmd := New().Command()
	if err := cmd.Flags().Set("csv", filepath.Join(t.TempDir(), "nao-existe.csv")); err != nil {
		t.Fatalf("cmd.Flags().Set(\"csv\", ...): %v", err)
	}

	fn, ok := cmd.GetFlagCompletionFunc("csv-levels")
	if !ok {
		t.Fatal("nenhuma função de completação registrada para --csv-levels")
	}

	got, directive := fn(cmd, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
	if len(got) != 0 {
		t.Errorf("completação de --csv-levels com arquivo inexistente = %v, esperava lista vazia", got)
	}
}
