// Package splitpdf implementa a ferramenta "split-pdf": separa um PDF em
// vários arquivos por página, por intervalos de páginas ou por uma
// expressão regular aplicada ao texto de cada página.
package splitpdf

// Options contém a configuração da ferramenta split-pdf. É o que é ligado
// às flags do cobra, perguntado no modo interativo e, por implementar
// tool.ProfileSupport, persistido como perfil salvo em YAML.
//
// Regex e Ranges são armazenados como string (não como *regexp.Regexp ou
// []string já processado) justamente porque Options precisa ser
// serializável em YAML; a conversão para os tipos que pdfutil espera
// acontece dentro de run().
type Options struct {
	// Input é o caminho do PDF a ser separado.
	Input string `yaml:"input"`
	// OutputDir é a pasta de saída dos arquivos gerados. Vazio significa
	// "mesma pasta do arquivo de entrada".
	OutputDir string `yaml:"output_dir"`
	// Mode é o critério de separação: "page", "range" ou "regex".
	Mode string `yaml:"mode"`
	// Ranges é a especificação crua de intervalos de páginas, ex.
	// "1-5,6-10,11-". Usado apenas quando Mode == "range".
	Ranges string `yaml:"ranges"`
	// Regex é a expressão regular crua aplicada ao texto de cada página.
	// Usado apenas quando Mode == "regex". Compilada dentro de run().
	Regex string `yaml:"regex"`
	// NameTemplate é o template usado para nomear os arquivos de saída
	// quando não há captura de regex disponível para nomeá-los.
	NameTemplate string `yaml:"name_template"`
	// Overwrite indica se arquivos de saída já existentes devem ser
	// sobrescritos.
	Overwrite bool `yaml:"overwrite"`
}

// defaultOptions devolve as Options padrão da ferramenta split-pdf: modo
// "page" (uma página por arquivo) e o template de nome "pagina-%03d".
//
// NameTemplate NÃO inclui a extensão ".pdf": quem monta o caminho final do
// arquivo de saída (pdfutil.Split) é responsável por acrescentá-la uma única
// vez, inclusive quando o usuário fornece o próprio template já com ".pdf".
func defaultOptions() Options {
	return Options{
		Mode:         "page",
		NameTemplate: "pagina-%03d",
	}
}
