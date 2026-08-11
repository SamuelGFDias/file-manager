package pdfutil

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// CSVMap é o mapeamento de chave para hierarquia de pastas, vindo de uma
// planilha (ver LoadCSVMap). É o caso de uso em que o usuário já tem uma
// planilha dizendo onde cada documento deve ser arquivado, e o PDF fornece
// só a chave para procurar nela — o inverso do modo --level, em que a
// hierarquia inteira é extraída de dentro do PDF por regex.
type CSVMap struct {
	// KeyColumn é o nome da coluna-chave efetivamente usada (a informada,
	// ou a primeira coluna do cabeçalho quando nenhuma foi informada).
	KeyColumn string
	// Levels são os nomes das colunas de hierarquia, na ordem em que
	// formam o caminho de destino.
	Levels []string
	// Rows mapeia cada chave (comparada como texto, com espaços das
	// pontas removidos — nunca convertida para número, já que zeros à
	// esquerda são significativos em número de nota) aos componentes de
	// pasta já normalizados (ver NormalizeComponent), na ordem de Levels.
	// Uma célula de nível vazia na planilha não gera um componente vazio
	// aqui: o componente é simplesmente omitido (ver Warnings).
	Rows map[string][]string
	// Order preserva a ordem de leitura das chaves no arquivo. Rows é um
	// map, sem ordem garantida — Order existe para que quem for mostrar
	// "a primeira linha da planilha" (ex: o resumo interativo em
	// internal/tools/organizepdf/screen.go) tenha como fazer isso de
	// forma determinística.
	Order []string
	// Warnings lista avisos não-fatais encontrados durante a leitura — o
	// único caso hoje é uma célula de nível vazia, identificando a chave
	// da linha e a coluna vazia. Nunca inclui erros: um problema que
	// impede a leitura por completo (planilha sem cabeçalho, coluna
	// inexistente, chave duplicada) vira o segundo valor de retorno de
	// LoadCSVMap, não um Warning.
	Warnings []string
}

// Lookup devolve os componentes de pasta associados à chave (espaços das
// pontas removidos antes de comparar, do mesmo jeito que LoadCSVMap
// armazenou as chaves lidas da planilha — assim uma chave extraída do PDF
// com espaço ao redor ainda casa). ok=false quando a chave não está na
// planilha.
func (m CSVMap) Lookup(key string) ([]string, bool) {
	v, ok := m.Rows[strings.TrimSpace(key)]
	return v, ok
}

// csvBOM é o Byte Order Mark UTF-8 (EF BB BF) que o Excel grava no início de
// um CSV; ver csvUTF8BOM em report.go, mesmo valor, escrito por nós ao
// gerar relatórios — plausível que uma planilha de entrada venha de lá ou
// diretamente do Excel. Sem descartar, o nome da primeira coluna do
// cabeçalho viria com esse lixo invisível grudado e nunca casaria com o
// nome informado em --csv-key-column/--csv-levels.
var csvBOM = []byte(csvUTF8BOM)

// LoadCSVMap lê a planilha em path e monta o CSVMap correspondente.
//
// keyColumn vazio usa a primeira coluna do cabeçalho como chave;
// levelColumns vazio usa todas as colunas exceto a chave, na ordem em que
// aparecem no arquivo. Um nome informado (em qualquer um dos dois) que não
// exista no cabeçalho é erro, listando as colunas disponíveis.
//
// O separador (vírgula ou ponto e vírgula — o Excel em português salva CSV
// com ";" por padrão) é detectado automaticamente a partir do cabeçalho: o
// separador que produzir mais colunas vence; empate com 1 coluna só (nenhum
// dos dois separadores encontrado) é erro.
//
// Uma chave duplicada na planilha é erro, citando a chave repetida: em
// contexto fiscal, duas linhas apontando para pastas diferentes sob o mesmo
// número de nota precisam ser corrigidas por quem preencheu a planilha, não
// resolvidas por sorteio. Uma célula de nível vazia, ao contrário, não é
// erro: o componente correspondente é omitido e um aviso é registrado em
// CSVMap.Warnings, identificando a chave e a coluna — uma célula em branco
// no meio da planilha não deve impedir o lote inteiro.
func LoadCSVMap(path, keyColumn string, levelColumns []string) (CSVMap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CSVMap{}, fmt.Errorf("ler planilha %q: %w", path, err)
	}
	data = bytes.TrimPrefix(data, csvBOM)

	if len(bytes.TrimSpace(data)) == 0 {
		return CSVMap{}, fmt.Errorf("planilha %q está vazia (sem linha de cabeçalho)", path)
	}

	sep, err := detectCSVSeparator(data)
	if err != nil {
		return CSVMap{}, fmt.Errorf("planilha %q: %w", path, err)
	}

	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = sep
	r.FieldsPerRecord = -1 // planilhas do Excel costumam vir com linhas de tamanho irregular

	records, err := r.ReadAll()
	if err != nil {
		return CSVMap{}, fmt.Errorf("ler planilha %q: %w", path, err)
	}
	if len(records) == 0 {
		return CSVMap{}, fmt.Errorf("planilha %q está vazia (sem linha de cabeçalho)", path)
	}

	header := make([]string, len(records[0]))
	for i, h := range records[0] {
		header[i] = strings.TrimSpace(h)
	}
	if len(header) < 2 {
		return CSVMap{}, fmt.Errorf(
			"planilha %q precisa ter um cabeçalho com pelo menos 2 colunas (chave + ao menos um nível de pasta), tem %d",
			path, len(header),
		)
	}

	keyIdx := 0
	keyColumn = strings.TrimSpace(keyColumn)
	if keyColumn != "" {
		idx := indexOfHeader(header, keyColumn)
		if idx < 0 {
			return CSVMap{}, fmt.Errorf(
				"coluna-chave %q não existe na planilha %q; colunas disponíveis: %s",
				keyColumn, path, strings.Join(header, ", "),
			)
		}
		keyIdx = idx
		keyColumn = header[idx]
	} else {
		keyColumn = header[0]
	}

	var levelIdxs []int
	var levelNames []string
	if len(levelColumns) == 0 {
		for i, h := range header {
			if i == keyIdx {
				continue
			}
			levelIdxs = append(levelIdxs, i)
			levelNames = append(levelNames, h)
		}
	} else {
		for _, lc := range levelColumns {
			lc = strings.TrimSpace(lc)
			idx := indexOfHeader(header, lc)
			if idx < 0 {
				return CSVMap{}, fmt.Errorf(
					"coluna de nível %q não existe na planilha %q; colunas disponíveis: %s",
					lc, path, strings.Join(header, ", "),
				)
			}
			levelIdxs = append(levelIdxs, idx)
			levelNames = append(levelNames, header[idx])
		}
	}

	m := CSVMap{
		KeyColumn: keyColumn,
		Levels:    levelNames,
		Rows:      make(map[string][]string, len(records)-1),
	}

	for _, record := range records[1:] {
		key := strings.TrimSpace(fieldAt(record, keyIdx))
		if key == "" {
			// Linha sem chave (ex: linha totalmente em branco, ou só com
			// separadores) — nada a indexar, ignorada silenciosamente.
			continue
		}
		if _, exists := m.Rows[key]; exists {
			return CSVMap{}, fmt.Errorf("chave %q duplicada na planilha %q", key, path)
		}

		components := make([]string, 0, len(levelIdxs))
		for i, idx := range levelIdxs {
			val := strings.TrimSpace(fieldAt(record, idx))
			if val == "" {
				m.Warnings = append(m.Warnings, fmt.Sprintf(
					"chave %q: coluna %q vazia, componente de pasta ignorado", key, levelNames[i],
				))
				continue
			}
			components = append(components, NormalizeComponent(val))
		}

		m.Rows[key] = components
		m.Order = append(m.Order, key)
	}

	return m, nil
}

// ReadCSVHeader devolve só o cabeçalho (nomes de coluna, já sem espaços nas
// pontas) de uma planilha, sem carregar as linhas de dados — usado pelo
// fluxo interativo (internal/tools/organizepdf/screen.go) para oferecer as
// colunas disponíveis quando o usuário quer escolher manualmente a
// coluna-chave ou as colunas de hierarquia. Mesma detecção de separador e
// descarte de BOM de LoadCSVMap.
func ReadCSVHeader(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ler planilha %q: %w", path, err)
	}
	data = bytes.TrimPrefix(data, csvBOM)

	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("planilha %q está vazia (sem linha de cabeçalho)", path)
	}

	sep, err := detectCSVSeparator(data)
	if err != nil {
		return nil, fmt.Errorf("planilha %q: %w", path, err)
	}

	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = sep
	r.FieldsPerRecord = -1

	record, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("ler cabeçalho da planilha %q: %w", path, err)
	}

	header := make([]string, len(record))
	for i, h := range record {
		header[i] = strings.TrimSpace(h)
	}
	if len(header) < 2 {
		return nil, fmt.Errorf(
			"planilha %q precisa ter um cabeçalho com pelo menos 2 colunas (chave + ao menos um nível de pasta), tem %d",
			path, len(header),
		)
	}
	return header, nil
}

// fieldAt devolve record[idx], ou "" se idx estiver fora dos limites — uma
// planilha exportada do Excel pode ter linhas mais curtas que o cabeçalho
// quando as últimas células de uma linha estão vazias (o Excel às vezes
// omite os separadores finais em vez de gravá-los vazios).
func fieldAt(record []string, idx int) string {
	if idx < 0 || idx >= len(record) {
		return ""
	}
	return record[idx]
}

// indexOfHeader devolve o índice de name em header (comparação exata, após
// TrimSpace já aplicado a ambos os lados pelos chamadores), ou -1 se não
// encontrado.
func indexOfHeader(header []string, name string) int {
	for i, h := range header {
		if h == name {
			return i
		}
	}
	return -1
}

// detectCSVSeparator decide entre vírgula e ponto e vírgula a partir da
// primeira linha de data (já sem o BOM): o Excel em português salva CSV com
// ";" por padrão, e é essa a planilha que o usuário mais provavelmente vai
// ter na mão, mas nada impede uma planilha genuína separada por vírgula.
// Vence o separador que produzir mais colunas ao interpretar só a primeira
// linha; empatar com 1 coluna cada (nenhum dos dois separadores presente) é
// erro claro, em vez de seguir adiante com uma planilha de coluna única sem
// sentido.
func detectCSVSeparator(data []byte) (rune, error) {
	line := firstLine(data)

	commaCols := countColumnsInLine(line, ',')
	semiCols := countColumnsInLine(line, ';')

	if commaCols <= 1 && semiCols <= 1 {
		return 0, fmt.Errorf(
			"não foi possível detectar o separador (vírgula ou ponto e vírgula) a partir do cabeçalho: %q",
			string(line),
		)
	}
	if semiCols > commaCols {
		return ';', nil
	}
	return ',', nil
}

// firstLine devolve a primeira linha de data, sem o terminador ("\n", ou
// "\r\n" — o \r sobra é removido explicitamente porque o corte abaixo só
// procura por "\n").
func firstLine(data []byte) []byte {
	if idx := bytes.IndexByte(data, '\n'); idx >= 0 {
		return bytes.TrimSuffix(data[:idx], []byte("\r"))
	}
	return data
}

// countColumnsInLine interpreta line como um único registro CSV separado
// por sep e devolve quantos campos resultaram; 0 se a linha não puder ser
// interpretada com esse separador (ex: aspas não fechadas) — tratado pelo
// chamador do mesmo jeito que "1 coluna", já que não é um separador válido
// para esta planilha.
func countColumnsInLine(line []byte, sep rune) int {
	r := csv.NewReader(bytes.NewReader(line))
	r.Comma = sep
	record, err := r.Read()
	if err != nil {
		return 0
	}
	return len(record)
}

// componentWhitespace casa espaços e tabulações — os caracteres que
// NormalizeComponent troca por "_" — para não confundir com outros
// caracteres de controle, que SanitizeFilename (chamada em seguida) já
// trata à parte.
var componentWhitespace = strings.NewReplacer(" ", "_", "\t", "_")

// repeatedUnderscores colapsa sequências de "_" (que podem surgir de vários
// espaços/tabulações seguidos, ou de um espaço adjacente a um "_" já
// presente no valor original) em um único "_".
var repeatedUnderscores = func() func(string) string {
	return collapseRuns('_')
}()

// collapseRuns devolve uma função que substitui qualquer sequência de dois
// ou mais r consecutivos por um único r.
func collapseRuns(r rune) func(string) string {
	return func(s string) string {
		var b strings.Builder
		b.Grow(len(s))
		prevWasR := false
		for _, c := range s {
			if c == r {
				if prevWasR {
					continue
				}
				prevWasR = true
			} else {
				prevWasR = false
			}
			b.WriteRune(c)
		}
		return b.String()
	}
}

// NormalizeComponent transforma um valor de célula da planilha em um nome
// de pasta seguro para uso em rede compartilhada e em ambiente misto
// Windows/Linux: remove acentos (decompõe em NFD, descarta as marcas de
// combinação — categoria Unicode Mn — e recompõe em NFC), troca espaços e
// tabulações por "_", colapsa "_" repetidos e por fim passa por
// SanitizeFilename para garantir que nenhum separador de caminho,
// sequência ".." ou caractere inválido em nome de arquivo no Windows
// sobreviva. Uma string cujo resultado final ficaria vazio (ex: entrada
// vazia, ou só espaços) vira "sem-valor" — uma pasta sem nome não é uma
// opção.
func NormalizeComponent(s string) string {
	decomposed := norm.NFD.String(s)

	var stripped strings.Builder
	stripped.Grow(len(decomposed))
	for _, r := range decomposed {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		stripped.WriteRune(r)
	}

	result := norm.NFC.String(stripped.String())
	result = componentWhitespace.Replace(result)
	result = repeatedUnderscores(result)
	result = SanitizeFilename(result)

	if result == "" {
		return "sem-valor"
	}
	return result
}
