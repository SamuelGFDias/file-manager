package pdfutil

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Level descreve um nível de pasta na hierarquia de organização, resolvido a
// partir de uma expressão regular aplicada ao texto extraído do PDF.
type Level struct {
	Label string
	Regex *regexp.Regexp
}

// Unmatched descreve por que um documento não pôde ser classificado.
type Unmatched struct {
	Level   string // rótulo do nível que falhou, ou "filename"
	Pattern string
}

// ResolveDestination calcula o caminho relativo de destino de um documento a
// partir do texto extraído dele, aplicando cada nível em ordem e, por fim, a
// expressão de nome de arquivo. Devolve unmatched != nil quando a
// classificação falha em algum ponto.
func ResolveDestination(text string, levels []Level, filenameRegex *regexp.Regexp) (relPath string, unmatched *Unmatched) {
	components := make([]string, 0, len(levels))

	for _, level := range levels {
		m := level.Regex.FindStringSubmatch(text)
		if m == nil {
			return "", &Unmatched{Level: level.Label, Pattern: level.Regex.String()}
		}
		value := m[0]
		if len(m) > 1 {
			value = m[1]
		}
		value = SanitizeFilename(value)
		if value == "" {
			return "", &Unmatched{Level: level.Label, Pattern: level.Regex.String()}
		}
		components = append(components, value)
	}

	var name string
	if filenameRegex != nil {
		m := filenameRegex.FindStringSubmatch(text)
		if m == nil {
			return "", &Unmatched{Level: "filename", Pattern: filenameRegex.String()}
		}
		value := m[0]
		if len(m) > 1 {
			value = m[1]
		}
		value = SanitizeFilename(value)
		if value == "" {
			return "", &Unmatched{Level: "filename", Pattern: filenameRegex.String()}
		}
		name = value
	}

	parts := append([]string{}, components...)
	if name != "" {
		// withPDFExt (definida em split.go, mesmo pacote) evita extensão
		// duplicada caso o grupo de captura de FilenameRegex já traga ".pdf"
		// no texto casado — mesma defesa aplicada em split.go.
		parts = append(parts, withPDFExt(name))
	}
	result := filepath.Join(parts...)

	if result != "" && (filepath.IsAbs(result) || result == ".." || strings.HasPrefix(result, ".."+string(filepath.Separator))) {
		return "", &Unmatched{Level: "destino", Pattern: "caminho resultante inválido"}
	}

	return result, nil
}

// ResolveDestinationCSV calcula o caminho relativo de destino de um
// documento cuja hierarquia de pastas vem de uma planilha (csvMap), em vez
// do conteúdo do PDF: keyRegex extrai a chave do texto (grupo de captura 1,
// ou o trecho inteiro casado sem grupo), csvMap.Lookup resolve a chave para
// os componentes de pasta, e o nome do arquivo é a própria chave — a menos
// que filenameRegex seja informado, caso em que ele vence, exatamente como
// em ResolveDestination.
//
// Duas formas de não-classificação são específicas deste modo, ambas com
// Unmatched.Level == "chave" (Unmatched.Pattern já vem pronto como a frase
// final em português, no mesmo padrão usado por "destino" — ver
// UnmatchedReason em report.go): a regex não casar com o texto do
// documento, e a chave encontrada não existir na planilha. Este segundo
// caso é, na prática, o mais frequente — por isso a mensagem cita a chave
// encontrada, para o usuário conferir na planilha.
func ResolveDestinationCSV(text string, csvMap CSVMap, keyRegex *regexp.Regexp, filenameRegex *regexp.Regexp) (relPath string, unmatched *Unmatched) {
	km := keyRegex.FindStringSubmatch(text)
	if km == nil {
		return "", &Unmatched{Level: "chave", Pattern: "chave não encontrada no documento"}
	}
	key := km[0]
	if len(km) > 1 {
		key = km[1]
	}
	key = strings.TrimSpace(key)

	components, ok := csvMap.Lookup(key)
	if !ok {
		return "", &Unmatched{Level: "chave", Pattern: fmt.Sprintf("chave %q não está na planilha", key)}
	}

	var name string
	if filenameRegex != nil {
		fm := filenameRegex.FindStringSubmatch(text)
		if fm == nil {
			return "", &Unmatched{Level: "filename", Pattern: filenameRegex.String()}
		}
		value := fm[0]
		if len(fm) > 1 {
			value = fm[1]
		}
		value = SanitizeFilename(value)
		if value == "" {
			return "", &Unmatched{Level: "filename", Pattern: filenameRegex.String()}
		}
		name = value
	} else {
		name = SanitizeFilename(key)
		if name == "" {
			name = "sem-valor"
		}
	}

	parts := append([]string{}, components...)
	parts = append(parts, withPDFExt(name))
	result := filepath.Join(parts...)

	if result != "" && (filepath.IsAbs(result) || result == ".." || strings.HasPrefix(result, ".."+string(filepath.Separator))) {
		return "", &Unmatched{Level: "destino", Pattern: "caminho resultante inválido"}
	}

	return result, nil
}

// RecordedEntry descreve um único arquivo efetivamente copiado ou movido
// por uma execução real (nunca uma simulação) de Organize, repassado a
// OrganizeOptions.Recorder para quem quiser persistir um histórico
// reversível (ver internal/history, que grava exatamente essas entradas).
type RecordedEntry struct {
	// Source é o caminho absoluto de origem do arquivo.
	Source string
	// Dest é o caminho absoluto de destino do arquivo.
	Dest string
	// Size é o tamanho do arquivo em Dest, lido logo após a operação.
	Size int64
}

// OrganizeOptions descreve os parâmetros de uma operação de organização de
// uma pasta de PDFs.
type OrganizeOptions struct {
	InputDir  string
	OutputDir string
	// Levels são os níveis de pasta calibrados por regex sobre o conteúdo
	// de cada PDF. Ignorado quando CSV não é nil: a hierarquia vem da
	// planilha, não do conteúdo — quem chama Organize (o comando
	// organize-pdf) já impede a combinação das duas flags antes de
	// chegar aqui, mas o núcleo se comporta de forma coerente mesmo
	// assim, para não depender só da validação de fora.
	Levels          []Level        // pode ser vazio => modo "somente renomear"
	FilenameRegex   *regexp.Regexp // se nil, o nome do arquivo é o original (ou a chave, em modo CSV)
	Copy            bool           // true = copia (default do CLI), false = move
	UnclassifiedDir string         // default "sem-classificacao"
	DryRun          bool
	Sample          int // 0 = todos; N>0 = só os N primeiros (ordem alfabética)
	Overwrite       bool
	Text            TextOptions // opções de extração de texto/OCR; zero-value = sem OCR
	// CSV, quando não-nil, faz a hierarquia de pastas vir de uma planilha
	// (ver LoadCSVMap) em vez do conteúdo do PDF: CSVKeyRegex extrai do
	// texto do PDF a chave usada para procurar a linha correspondente em
	// CSV, e os componentes de pasta daquela linha (já normalizados)
	// formam o caminho de destino. Levels é ignorado neste modo.
	CSV *CSVMap
	// CSVKeyRegex extrai, do texto do PDF, a chave usada para procurar em
	// CSV.Lookup. Só é usado (e deve ser não-nil) quando CSV não é nil; o
	// grupo de captura 1 vira a chave, ou o trecho inteiro casado quando
	// a regex não tem grupo de captura.
	CSVKeyRegex *regexp.Regexp
	// Recorder, quando não-nil, é chamado ao final de uma execução REAL
	// (nunca em DryRun) que tenha efetivamente copiado ou movido pelo
	// menos um arquivo, com action = "copy" ou "move" (espelhando Copy) e
	// entries = todos os arquivos efetivamente tocados, incluindo os que
	// foram para UnclassifiedDir. É o ponto de injeção usado por quem
	// chama Organize para gravar um histórico reversível, sem que este
	// pacote precise conhecer internal/history nem internal/config —
	// pdfutil permanece lógica pura de organização de arquivos, e quem
	// monta o Recorder (o comando organize-pdf) decide onde e como
	// persistir. Um erro devolvido por Recorder NUNCA falha Organize: a
	// operação de organizar já aconteceu e não pode ser desfeita por uma
	// falha em gravar o histórico dela; o erro vira um aviso em
	// OrganizeResult.Warnings. nil (o zero-value) desliga a gravação por
	// completo — é o comportamento de quem chama Organize diretamente,
	// como os testes deste pacote.
	Recorder func(action string, entries []RecordedEntry) error
}

// OrganizeEntry descreve o destino calculado (ou tentado) para um único
// arquivo.
type OrganizeEntry struct {
	Source    string
	Dest      string     // caminho relativo ao OutputDir
	Unmatched *Unmatched // nil quando classificado com sucesso
}

// OrganizeResult descreve o resultado de uma operação de organização.
type OrganizeResult struct {
	Organized    []OrganizeEntry
	Unclassified []OrganizeEntry
	DryRun       bool
	Total        int
	// Warnings contém avisos que não impedem a operação (que já
	// aconteceu) de ser considerada bem-sucedida — hoje, só a falha ao
	// gravar o histórico via OrganizeOptions.Recorder.
	Warnings []string
}

// Summary devolve um resumo textual curto do resultado da organização.
func (r OrganizeResult) Summary() string {
	prefix := ""
	if r.DryRun {
		prefix = "[simulação] "
	}
	return fmt.Sprintf("%s%d de %d arquivos organizados, %d em sem-classificacao", prefix, len(r.Organized), r.Total, len(r.Unclassified))
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("abrir %q: %w", src, err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("criar %q: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("copiar %q para %q: %w", src, dst, err)
	}
	return out.Close()
}

func moveOrCopyFile(src, dst string, copy bool) error {
	if dir := filepath.Dir(dst); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("criar diretório de destino %q: %w", dir, err)
		}
	}

	if copy {
		return copyFile(src, dst)
	}

	if err := os.Rename(src, dst); err != nil {
		// Rename falha entre sistemas de arquivos diferentes (ex: pendrive
		// para disco); cai para copiar + remover.
		if cErr := copyFile(src, dst); cErr != nil {
			return cErr
		}
		return os.Remove(src)
	}
	return nil
}

// destinationClaimed reporta se destAbs já está tomado, de uma das duas
// formas que Organize precisa tratar como colisão: já foi atribuído a um
// arquivo anterior do MESMO lote (assigned — checagem em memória, que por
// isso funciona igualmente em dry-run e em execução real, já que dry-run
// nunca grava nada em disco), ou já existe em disco, sobrevivente de uma
// execução anterior. overwrite=true desliga as duas checagens: com ela
// ligada, a intenção de quem chama é explícita — o último arquivo escrito
// vence —, então não há colisão a reportar.
//
// Um erro de os.Stat que não seja "não existe" (ex: permissão negada num
// diretório intermediário) é propagado para o chamador em vez de ser
// tratado como "não colide": mascarar esse tipo de erro faria Organize
// classificar um arquivo com base numa checagem que na verdade falhou.
func destinationClaimed(destAbs string, assigned map[string]bool, overwrite bool) (bool, error) {
	if overwrite {
		return false, nil
	}
	if assigned[destAbs] {
		return true, nil
	}
	if _, err := os.Stat(destAbs); err == nil {
		return true, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	return false, nil
}

// Organize classifica e move/copia os PDFs de InputDir para OutputDir de
// acordo com Levels e FilenameRegex.
func Organize(ctx context.Context, opts OrganizeOptions) (OrganizeResult, error) {
	if err := ctx.Err(); err != nil {
		return OrganizeResult{}, err
	}

	if opts.InputDir == "" {
		return OrganizeResult{}, fmt.Errorf("diretório de entrada não informado")
	}

	if opts.CSV != nil && opts.CSVKeyRegex == nil {
		return OrganizeResult{}, fmt.Errorf("CSVKeyRegex não pode ser nil quando CSV está definido")
	}

	dirEntries, err := os.ReadDir(opts.InputDir)
	if err != nil {
		return OrganizeResult{}, fmt.Errorf("ler diretório de entrada %q: %w", opts.InputDir, err)
	}

	var files []string
	for _, e := range dirEntries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".pdf") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	if opts.Sample > 0 && opts.Sample < len(files) {
		files = files[:opts.Sample]
	}

	unclassifiedDir := opts.UnclassifiedDir
	if unclassifiedDir == "" {
		unclassifiedDir = "sem-classificacao"
	}

	result := OrganizeResult{DryRun: opts.DryRun, Total: len(files)}

	// recorded acumula uma RecordedEntry por arquivo efetivamente
	// copiado/movido (classificado ou não), na ordem em que a operação
	// aconteceu. Permanece vazio em DryRun, já que o bloco que a alimenta
	// só roda quando !opts.DryRun (ver abaixo).
	var recorded []RecordedEntry

	// assignedDest acumula, à medida que o lote é processado, o destino
	// (join de OutputDir + Dest) de cada arquivo já classificado — em
	// AMBOS os modos, dry-run e execução real. Existe para que dois
	// arquivos do lote que resolvam para o mesmo destino (nota fiscal
	// duplicada na pasta de entrada, mesmo número de nota em fornecedores
	// diferentes — coisas do dia a dia de quem organiza nota fiscal) sejam
	// detectados do MESMO jeito nos dois modos. Antes desta checagem
	// explícita, a execução real só pegava essa colisão por acidente:
	// como o primeiro arquivo já tinha sido gravado em disco quando o
	// segundo chegava, o os.Stat de baixo (pensado para colisão com uma
	// execução ANTERIOR) também pegava esse caso por tabela — e a
	// simulação, que nunca grava nada, nunca via essa colisão. Resultado:
	// o relatório de --dry-run podia prometer uma classificação que a
	// execução real desmentia, o que destrói o valor da própria feature
	// de relatório (ver internal/pdfutil/report.go).
	assignedDest := make(map[string]bool, len(files))

	for _, name := range files {
		if err := ctx.Err(); err != nil {
			return OrganizeResult{}, err
		}

		srcPath := filepath.Join(opts.InputDir, name)

		var unmatched *Unmatched
		var dest string

		text, textErr := ExtractTextOpts(ctx, srcPath, opts.Text)
		if textErr != nil {
			unmatched = &Unmatched{Level: "texto", Pattern: "falha ao extrair texto"}
		} else if opts.CSV != nil {
			relPath, um := ResolveDestinationCSV(text, *opts.CSV, opts.CSVKeyRegex, opts.FilenameRegex)
			if um != nil {
				unmatched = um
			} else {
				dest = relPath
			}
		} else {
			relPath, um := ResolveDestination(text, opts.Levels, opts.FilenameRegex)
			if um != nil {
				unmatched = um
			} else {
				if opts.FilenameRegex == nil {
					relPath = filepath.Join(relPath, name)
				}
				dest = relPath
			}
		}

		// Colisão de destino: checada da MESMA forma em dry-run e em
		// execução real, ANTES de qualquer gravação — nunca depois. Cobre
		// as duas formas de colisão (ver destinationClaimed):
		// assignedDest (outro arquivo deste mesmo lote já reivindicou
		// este destino) e o destino já existir em disco, de uma execução
		// anterior. --overwrite desliga as duas: com ela ligada a
		// intenção já é explícita — o último arquivo escrito vence, sem
		// erro.
		if unmatched == nil {
			destAbs := filepath.Join(opts.OutputDir, dest)
			claimed, statErr := destinationClaimed(destAbs, assignedDest, opts.Overwrite)
			if statErr != nil {
				return OrganizeResult{}, fmt.Errorf("verificar destino de %q: %w", srcPath, statErr)
			}
			if claimed {
				unmatched = &Unmatched{Level: "destino", Pattern: fmt.Sprintf("destino já existe: %s", destAbs)}
			}
		}

		if unmatched != nil {
			dest = filepath.Join(unclassifiedDir, name)
		} else {
			assignedDest[filepath.Join(opts.OutputDir, dest)] = true
		}

		entry := OrganizeEntry{Source: srcPath, Dest: dest, Unmatched: unmatched}

		if !opts.DryRun {
			destAbs := filepath.Join(opts.OutputDir, dest)

			proceed := true
			if !opts.Overwrite {
				if _, statErr := os.Stat(destAbs); statErr == nil {
					// Colisão persiste mesmo dentro de --unclassified-dir
					// (ex: já havia um arquivo de mesmo nome lá, de uma
					// execução anterior). Isto só decide se a GRAVAÇÃO
					// acontece — não muda a classificação nem o relatório
					// —, então não precisa (nem faz sentido) rodar em
					// dry-run, que nunca grava nada.
					proceed = false
				}
			}

			if proceed {
				if err := moveOrCopyFile(srcPath, destAbs, opts.Copy); err != nil {
					return OrganizeResult{}, fmt.Errorf("organizar %q: %w", srcPath, err)
				}

				if opts.Recorder != nil {
					recorded = append(recorded, RecordedEntry{
						Source: absPathOrSelf(srcPath),
						Dest:   absPathOrSelf(destAbs),
						Size:   fileSizeOrZero(destAbs),
					})
				}
			}
		}

		if unmatched != nil {
			result.Unclassified = append(result.Unclassified, entry)
		} else {
			result.Organized = append(result.Organized, entry)
		}
	}

	// A gravação do histórico só acontece depois que TODOS os arquivos já
	// foram efetivamente organizados: recorded fica vazio em DryRun (o
	// bloco acima nunca roda) e também quando nada foi de fato
	// copiado/movido (ex: pasta de entrada vazia, ou toda colisão
	// resolvida sem sobrescrever) — uma execução vazia não é histórico.
	if opts.Recorder != nil && len(recorded) > 0 {
		action := "copy"
		if !opts.Copy {
			action = "move"
		}
		if recErr := opts.Recorder(action, recorded); recErr != nil {
			// Falha ao gravar o manifesto NUNCA falha Organize: a
			// operação de organizar já aconteceu de verdade, e desfazer
			// esse resultado (ou reportar erro sobre uma operação
			// concluída) confundiria mais do que perder o histórico
			// desta execução específica.
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"não foi possível gravar o histórico desta operação (desfazer ficará indisponível para ela): %v", recErr,
			))
		}
	}

	return result, nil
}

// absPathOrSelf devolve o caminho absoluto de p; se filepath.Abs falhar
// (extremamente raro — só quando os.Getwd falha), devolve p sem alteração
// em vez de propagar o erro, já que RecordedEntry não pode impedir a
// operação de organizar (que já terminou) de ser reportada como concluída.
func absPathOrSelf(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

// fileSizeOrZero devolve o tamanho de p, ou 0 se não for possível ler
// (mesmo raciocínio de absPathOrSelf: nunca falha Organize por causa de um
// detalhe do registro histórico).
func fileSizeOrZero(p string) int64 {
	info, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return info.Size()
}
