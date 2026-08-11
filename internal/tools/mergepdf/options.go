// Package mergepdf implementa a ferramenta "merge-pdf": une vários arquivos
// PDF (informados diretamente ou coletados a partir de pastas) em um único
// arquivo de saída.
package mergepdf

// Options descreve a configuração de uma execução de merge-pdf. É o que é
// vinculado às flags do cobra, perguntado nos prompts interativos e, via
// Profile, persistido como perfil salvo.
type Options struct {
	// Inputs são os arquivos PDF e/ou pastas a incluir na união. Pastas são
	// varridas conforme MaxDepth.
	Inputs []string `yaml:"inputs"`
	// MaxDepth controla a profundidade de varredura de pastas em Inputs:
	// 0 = só a pasta informada; N = desce N níveis; -1 = ilimitado.
	MaxDepth int `yaml:"max_depth"`
	// Output é o caminho do PDF resultante.
	Output string `yaml:"output"`
	// Sort é a ordem de união dos arquivos: "name" ou "mtime".
	Sort string `yaml:"sort"`
	// Overwrite permite sobrescrever Output caso já exista.
	Overwrite bool `yaml:"overwrite"`
}

// defaultOptions devolve as Options padrão de merge-pdf.
func defaultOptions() Options {
	return Options{
		MaxDepth: 1,
		Sort:     "name",
	}
}
