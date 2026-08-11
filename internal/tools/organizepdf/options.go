// Package organizepdf implementa a ferramenta "organize-pdf": classifica os
// PDFs de uma pasta numa hierarquia de pastas de destino, com base em
// valores encontrados no texto de cada arquivo, e opcionalmente renomeia
// cada um pelo valor capturado. Caso de uso motivador: uma pasta cheia de
// notas fiscais vira destino/FORNECEDOR/FILIAL/0001.pdf.
package organizepdf

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/SamuelGFDias/file-manager/internal/pdfutil"
)

// LevelSpec é um nível de pasta configurado, na forma serializável em YAML
// (a regex fica como string crua; a compilação para *regexp.Regexp
// acontece em BuildLevels, dentro de run()).
type LevelSpec struct {
	// Label identifica o nível (ex: "fornecedor"), usado como rótulo nas
	// perguntas de calibração e nas mensagens de arquivo não classificado.
	Label string `yaml:"label"`
	// Regex é a expressão regular crua aplicada ao texto extraído do PDF.
	// O primeiro grupo de captura (se houver) vira o nome do componente de
	// pasta; sem grupo de captura, o próprio trecho casado é usado.
	Regex string `yaml:"regex"`
}

// Options contém a configuração da ferramenta organize-pdf. É o que é
// ligado às flags do cobra, perguntado no modo interativo e, por
// implementar tool.ProfileSupport, persistido como perfil salvo em YAML.
type Options struct {
	// InputDir é a pasta com os PDFs a organizar.
	InputDir string `yaml:"input_dir"`
	// OutputDir é a pasta de destino da hierarquia organizada.
	OutputDir string `yaml:"output_dir"`
	// Levels são os níveis de pasta, na ordem em que definem a hierarquia
	// de destino. Vazio = modo "somente renomear": os arquivos vão direto
	// para OutputDir, sem subpastas.
	Levels []LevelSpec `yaml:"levels"`
	// FilenameRegex é a expressão regular crua cujo grupo de captura vira o
	// nome do arquivo de destino. Vazio = mantém o nome original.
	FilenameRegex string `yaml:"filename_regex"`
	// Move indica se os arquivos devem ser movidos em vez de copiados. O
	// padrão (false) é copiar, preservando os arquivos de origem.
	Move bool `yaml:"move"`
	// UnclassifiedDir é a subpasta, dentro de OutputDir, para onde vão os
	// arquivos que não casaram com algum nível ou com FilenameRegex.
	UnclassifiedDir string `yaml:"unclassified_dir"`
	// Overwrite permite sobrescrever arquivos já existentes no destino.
	Overwrite bool `yaml:"overwrite"`
	// DryRun só simula a organização, sem copiar nem mover nada. Nunca é
	// persistido no perfil: é sempre decidido na hora da execução.
	DryRun bool `yaml:"-"`
	// Sample limita a simulação aos N primeiros arquivos (0 = todos). Assim
	// como DryRun, nunca é persistido no perfil.
	Sample int `yaml:"-"`
}

// defaultOptions devolve as Options padrão de organize-pdf: sem níveis
// (modo "somente renomear" até que o usuário adicione algum), copiando
// (não destrutivo) para "sem-classificacao" quando um arquivo não casa.
func defaultOptions() Options {
	return Options{
		UnclassifiedDir: "sem-classificacao",
	}
}

// ParseLevelFlags converte a entrada crua da flag repetível --level, no
// formato "rótulo=regex", em []LevelSpec. É uma função pura, sem
// dependência de I/O, para ser facilmente testável.
//
// Cada string é dividida apenas no PRIMEIRO "=" (strings.SplitN com limite
// 2): a regex do lado direito quase sempre contém "=" (ex: em asserções
// como "TOTAL\s*=\s*(\d+)"), e dividir em todas as ocorrências quebraria a
// regex ao meio.
//
// Entrada nil ou vazia devolve um slice vazio sem erro — é o modo "somente
// renomear", uma configuração válida e esperada.
func ParseLevelFlags(raw []string) ([]LevelSpec, error) {
	if len(raw) == 0 {
		return []LevelSpec{}, nil
	}

	specs := make([]LevelSpec, 0, len(raw))
	for _, entry := range raw {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("nível de pasta inválido %q: use o formato \"rótulo=regex\"", entry)
		}

		label, pattern := parts[0], parts[1]
		if label == "" {
			return nil, fmt.Errorf("nível de pasta inválido %q: rótulo vazio", entry)
		}
		if pattern == "" {
			return nil, fmt.Errorf("nível de pasta inválido %q: regex vazia", entry)
		}

		specs = append(specs, LevelSpec{Label: label, Regex: pattern})
	}

	return specs, nil
}

// BuildLevels compila os LevelSpec em []pdfutil.Level, na mesma ordem. É
// uma função pura, testável sem depender de arquivos em disco.
//
// Slice de entrada vazio (ou nil) devolve um slice vazio sem erro. Quando
// uma regex não compila, o erro identifica QUAL rótulo de nível é o
// problemático, para que o usuário saiba exatamente o que corrigir.
func BuildLevels(specs []LevelSpec) ([]pdfutil.Level, error) {
	if len(specs) == 0 {
		return []pdfutil.Level{}, nil
	}

	levels := make([]pdfutil.Level, 0, len(specs))
	for _, spec := range specs {
		re, err := regexp.Compile(spec.Regex)
		if err != nil {
			return nil, fmt.Errorf("nível %q: regex inválida: %w", spec.Label, err)
		}
		levels = append(levels, pdfutil.Level{Label: spec.Label, Regex: re})
	}

	return levels, nil
}
