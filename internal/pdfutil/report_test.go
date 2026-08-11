package pdfutil

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// buildSampleResult monta um OrganizeResult com 2 arquivos classificados e
// 2 não classificados (um por "filename", um por nível nomeado), usado por
// vários testes deste arquivo.
func buildSampleResult() OrganizeResult {
	return OrganizeResult{
		Organized: []OrganizeEntry{
			{Source: "/entrada/b.pdf", Dest: "fornecedorB/0001.pdf"},
			{Source: "/entrada/a.pdf", Dest: "fornecedorA/0002.pdf"},
		},
		Unclassified: []OrganizeEntry{
			{Source: "/entrada/c.pdf", Dest: "sem-classificacao/c.pdf", Unmatched: &Unmatched{Level: "fornecedor", Pattern: `FORNECEDOR:\s*(\w+)`}},
			{Source: "/entrada/d.pdf", Dest: "sem-classificacao/d.pdf", Unmatched: &Unmatched{Level: "filename", Pattern: `NF:\s*(\d+)`}},
		},
		Total: 4,
	}
}

func TestBuildReportIncludesClassifiedAndUnclassified(t *testing.T) {
	rows := BuildReport(buildSampleResult())

	if len(rows) != 4 {
		t.Fatalf("BuildReport() devolveu %d linhas, esperava 4 (2 classificados + 2 não classificados)", len(rows))
	}

	classificados, naoClassificados := 0, 0
	for _, r := range rows {
		if r.Classificado {
			classificados++
			if r.Motivo != "" {
				t.Errorf("linha classificada %q tem Motivo não vazio: %q", r.Arquivo, r.Motivo)
			}
		} else {
			naoClassificados++
			if r.Motivo == "" {
				t.Errorf("linha não classificada %q deveria ter Motivo preenchido", r.Arquivo)
			}
		}
	}
	if classificados != 2 || naoClassificados != 2 {
		t.Errorf("classificados=%d, naoClassificados=%d, esperava 2 e 2", classificados, naoClassificados)
	}
}

// TestBuildReportDeterministicOrder embaralha a ordem de Organized e
// Unclassified na entrada e confirma que a saída de BuildReport é sempre a
// mesma (ordenada por Arquivo) — um relatório cuja ordem varia entre
// execuções do mesmo lote seria inútil para comparar duas rodadas lado a
// lado.
func TestBuildReportDeterministicOrder(t *testing.T) {
	base := buildSampleResult()
	want := BuildReport(base)

	rnd := rand.New(rand.NewSource(42))

	for i := 0; i < 5; i++ {
		shuffled := OrganizeResult{Total: base.Total}
		shuffled.Organized = append([]OrganizeEntry{}, base.Organized...)
		shuffled.Unclassified = append([]OrganizeEntry{}, base.Unclassified...)
		rnd.Shuffle(len(shuffled.Organized), func(a, b int) {
			shuffled.Organized[a], shuffled.Organized[b] = shuffled.Organized[b], shuffled.Organized[a]
		})
		rnd.Shuffle(len(shuffled.Unclassified), func(a, b int) {
			shuffled.Unclassified[a], shuffled.Unclassified[b] = shuffled.Unclassified[b], shuffled.Unclassified[a]
		})

		got := BuildReport(shuffled)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("iteração %d: BuildReport() com entrada embaralhada devolveu ordem diferente.\ngot:  %+v\nwant: %+v", i, got, want)
		}
	}
}

func TestUnmatchedReasonMessages(t *testing.T) {
	tests := []struct {
		name string
		u    *Unmatched
		want string
	}{
		{"nível nomeado", &Unmatched{Level: "fornecedor", Pattern: `X`}, `nível "fornecedor" não encontrado`},
		{"filename", &Unmatched{Level: "filename", Pattern: `X`}, "nome do arquivo não encontrado"},
		{"texto", &Unmatched{Level: "texto", Pattern: "falha ao extrair texto"}, "não foi possível extrair texto do arquivo"},
		{"destino já existe", &Unmatched{Level: "destino", Pattern: "destino já existe: /abs/ACME/0001.pdf"}, "destino já existe: /abs/ACME/0001.pdf"},
		{"destino caminho inválido", &Unmatched{Level: "destino", Pattern: "caminho resultante inválido"}, "caminho resultante inválido"},
		{"nil", nil, "motivo desconhecido"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UnmatchedReason(tt.u)
			if got != tt.want {
				t.Errorf("UnmatchedReason(%+v) = %q, want %q", tt.u, got, tt.want)
			}
		})
	}
}

func TestWriteReportCSVHeaderAndBOM(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteReportCSV(&buf, nil); err != nil {
		t.Fatalf("WriteReportCSV() erro inesperado: %v", err)
	}

	out := buf.String()
	if !strings.HasPrefix(out, csvUTF8BOM) {
		t.Fatalf("CSV gerado não começa com o BOM UTF-8; primeiros bytes: %q", out[:min(10, len(out))])
	}

	withoutBOM := strings.TrimPrefix(out, csvUTF8BOM)
	r := csv.NewReader(strings.NewReader(withoutBOM))
	header, err := r.Read()
	if err != nil {
		t.Fatalf("erro ao ler cabeçalho de volta: %v", err)
	}
	want := []string{"arquivo", "origem", "destino", "classificado", "motivo"}
	if !reflect.DeepEqual(header, want) {
		t.Errorf("cabeçalho = %v, want %v", header, want)
	}
}

// TestWriteReportCSVRoundTripSpecialChars é o teste crítico do CSV: um
// campo com vírgula, aspas e acento — bem realista para um caminho de
// arquivo ou nome de fornecedor — precisa sobreviver ao round-trip via
// encoding/csv sem corromper as colunas vizinhas.
func TestWriteReportCSVRoundTripSpecialChars(t *testing.T) {
	rows := []ReportRow{
		{
			Arquivo:      `nota "especial", com vírgula.pdf`,
			Origem:       `/entrada/nota "especial", com vírgula.pdf`,
			Destino:      "fornecedor Ção/0001.pdf",
			Classificado: true,
		},
		{
			Arquivo:      "nao-classificado.pdf",
			Origem:       "/entrada/nao-classificado.pdf",
			Destino:      "sem-classificacao/nao-classificado.pdf",
			Classificado: false,
			Motivo:       `nível "fornecedor" não encontrado`,
		},
	}

	var buf bytes.Buffer
	if err := WriteReportCSV(&buf, rows); err != nil {
		t.Fatalf("WriteReportCSV() erro inesperado: %v", err)
	}

	withoutBOM := strings.TrimPrefix(buf.String(), csvUTF8BOM)
	r := csv.NewReader(strings.NewReader(withoutBOM))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("erro ao ler CSV de volta: %v", err)
	}
	if len(records) != 3 { // cabeçalho + 2 linhas
		t.Fatalf("esperava 3 registros (cabeçalho + 2 linhas), obteve %d: %+v", len(records), records)
	}

	first := records[1]
	if first[0] != rows[0].Arquivo {
		t.Errorf("arquivo = %q, want %q", first[0], rows[0].Arquivo)
	}
	if first[1] != rows[0].Origem {
		t.Errorf("origem = %q, want %q", first[1], rows[0].Origem)
	}
	if first[2] != rows[0].Destino {
		t.Errorf("destino = %q, want %q", first[2], rows[0].Destino)
	}
	if first[3] != "sim" {
		t.Errorf(`classificado = %q, want "sim"`, first[3])
	}

	second := records[2]
	if second[3] != "nao" {
		t.Errorf(`classificado = %q, want "nao"`, second[3])
	}
	if second[4] != rows[1].Motivo {
		t.Errorf("motivo = %q, want %q", second[4], rows[1].Motivo)
	}
}

func TestWriteReportJSONRoundTrip(t *testing.T) {
	rows := BuildReport(buildSampleResult())

	var buf bytes.Buffer
	if err := WriteReportJSON(&buf, rows); err != nil {
		t.Fatalf("WriteReportJSON() erro inesperado: %v", err)
	}

	var decoded []ReportRow
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("JSON gerado não decodifica de volta: %v\nconteúdo:\n%s", err, buf.String())
	}

	if !reflect.DeepEqual(decoded, rows) {
		t.Errorf("JSON decodificado difere do original.\ngot:  %+v\nwant: %+v", decoded, rows)
	}
}
