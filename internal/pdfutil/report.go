package pdfutil

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
)

// ReportRow é uma linha do relatório de uma execução de Organize: uma
// entrada por arquivo considerado, classificado ou não. O caso motivador é
// fiscal (lotes de notas fiscais): é preciso poder conferir depois, numa
// planilha, por que cada arquivo foi parar onde foi.
type ReportRow struct {
	// Arquivo é o nome base do arquivo de origem (ex: "nota123.pdf").
	Arquivo string `json:"arquivo"`
	// Origem é o caminho absoluto (ou o caminho como recebido em
	// OrganizeEntry.Source) de origem do arquivo.
	Origem string `json:"origem"`
	// Destino é o caminho relativo ao OutputDir da execução; vazio quando o
	// arquivo não foi classificado.
	Destino string `json:"destino"`
	// Classificado indica se o arquivo casou com todos os níveis (e com
	// FilenameRegex, quando informado).
	Classificado bool `json:"classificado"`
	// Motivo é vazio quando Classificado, e descreve em português por que a
	// classificação falhou caso contrário.
	Motivo string `json:"motivo"`
}

// BuildReport monta as linhas do relatório de uma execução a partir do seu
// OrganizeResult, incluindo TANTO os arquivos classificados quanto os não
// classificados — um relatório que mostrasse só um lado não serviria para
// auditar o lote inteiro. É uma função pura, sem I/O: só WriteReportCSV e
// WriteReportJSON tocam em disco.
//
// As linhas saem ordenadas por Arquivo, não na ordem de processamento de
// Organize (que é apenas a ordem alfabética de leitura do diretório, mas
// intercalada entre Organized e Unclassified): um relatório cuja ordem
// pudesse variar entre duas execuções do mesmo lote seria inútil para
// comparar duas rodadas lado a lado, ex.: numa planilha.
func BuildReport(r OrganizeResult) []ReportRow {
	rows := make([]ReportRow, 0, len(r.Organized)+len(r.Unclassified))

	for _, e := range r.Organized {
		rows = append(rows, ReportRow{
			Arquivo:      filepath.Base(e.Source),
			Origem:       e.Source,
			Destino:      e.Dest,
			Classificado: true,
		})
	}
	for _, e := range r.Unclassified {
		rows = append(rows, ReportRow{
			Arquivo:      filepath.Base(e.Source),
			Origem:       e.Source,
			Destino:      e.Dest,
			Classificado: false,
			Motivo:       UnmatchedReason(e.Unmatched),
		})
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Arquivo < rows[j].Arquivo })

	return rows
}

// UnmatchedReason traduz um Unmatched no motivo legível em português usado
// tanto na coluna "motivo" do relatório (BuildReport) quanto na linha de
// detalhe de cada arquivo não classificado mostrada na tela pelo comando
// organize-pdf (internal/tools/organizepdf) — fonte única, para que as duas
// jamais divirjam na redação. Cobre os valores de Unmatched.Level que
// Organize de fato produz (ver organize.go): o rótulo de um nível
// configurado pelo usuário (ex: "fornecedor"), "filename" (falha em
// FilenameRegex), "texto" (falha ao extrair texto do PDF) e "destino"
// (colisão de destino ou caminho resultante inválido — Unmatched.Pattern já
// vem como uma frase legível em português nesse caso, ex: "destino já
// existe: /abs/ACME/0001.pdf" ou "caminho resultante inválido"; "destino"
// é uma pseudo-etiqueta interna, não um nível calibrado pelo usuário, por
// isso NÃO passa pelo formato "nível %q não encontrado" do caso default —
// fazer isso confundiria quem lê achando que errou a calibração de um
// nível chamado "destino"). u == nil não deveria acontecer para uma
// entrada não classificada, mas é tratado por segurança: uma linha "não
// classificado" sem motivo nenhum seria pior do que um motivo genérico.
func UnmatchedReason(u *Unmatched) string {
	if u == nil {
		return "motivo desconhecido"
	}
	switch u.Level {
	case "filename":
		return "nome do arquivo não encontrado"
	case "texto":
		return "não foi possível extrair texto do arquivo"
	case "destino":
		return u.Pattern
	default:
		return fmt.Sprintf("nível %q não encontrado", u.Level)
	}
}

// csvUTF8BOM é o Byte Order Mark UTF-8 (EF BB BF), escrito no início de todo
// CSV gerado por WriteReportCSV.
//
// Decisão deliberada: o público desta ferramenta abre o relatório dando
// duplo-clique no Excel em português, e o Excel só reconhece um CSV UTF-8
// SEM BOM como Windows-1252 — o resultado é que nomes de fornecedor com
// acento, "não" em "classificado" etc. aparecem corrompidos ao abrir o
// arquivo. Com o BOM, o Excel detecta UTF-8 corretamente. Outros programas
// (LibreOffice, Google Sheets, e o próprio encoding/csv ao ler o arquivo de
// volta) ignoram ou toleram o BOM sem problema — só custa três bytes.
const csvUTF8BOM = "\ufeff"

// reportCSVHeader é o cabeçalho, em português, de todo CSV gerado por
// WriteReportCSV.
var reportCSVHeader = []string{"arquivo", "origem", "destino", "classificado", "motivo"}

// WriteReportCSV grava rows em w como CSV, com BOM UTF-8 no início (ver
// csvUTF8BOM) e cabeçalho reportCSVHeader. A coluna "classificado" sai como
// "sim"/"nao" em vez de "true"/"false" — quem lê essa coluna é uma pessoa
// numa planilha, não um programa (para consumo por programa, ver
// WriteReportJSON).
//
// Usa encoding/csv em vez de concatenar strings: caminho de arquivo com
// vírgula, aspas ou acento é um caso normal aqui (nome de fornecedor,
// pasta com espaço), e escapar isso à mão seria reimplementar, com bugs, o
// que a stdlib já resolve corretamente.
func WriteReportCSV(w io.Writer, rows []ReportRow) error {
	if _, err := io.WriteString(w, csvUTF8BOM); err != nil {
		return fmt.Errorf("escrever BOM do relatório CSV: %w", err)
	}

	cw := csv.NewWriter(w)

	if err := cw.Write(reportCSVHeader); err != nil {
		return fmt.Errorf("escrever cabeçalho do relatório CSV: %w", err)
	}

	for _, r := range rows {
		classificado := "nao"
		if r.Classificado {
			classificado = "sim"
		}
		if err := cw.Write([]string{r.Arquivo, r.Origem, r.Destino, classificado, r.Motivo}); err != nil {
			return fmt.Errorf("escrever linha do relatório CSV (%s): %w", r.Arquivo, err)
		}
	}

	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("gravar relatório CSV: %w", err)
	}
	return nil
}

// WriteReportJSON grava rows em w como JSON indentado (2 espaços), um
// array de objetos com os mesmos campos de ReportRow — para quem for
// consumir o relatório por programa em vez de abrir numa planilha.
func WriteReportJSON(w io.Writer, rows []ReportRow) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rows); err != nil {
		return fmt.Errorf("gravar relatório JSON: %w", err)
	}
	return nil
}
