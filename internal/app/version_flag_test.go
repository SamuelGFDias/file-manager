package app

import (
	"strings"
	"testing"
)

// runRootCapturing executa o comando raiz com args, capturando tudo que foi
// escrito em os.Stdout (via captureStdout, definido em undo_test.go).
// Necessário porque o subcomando "version" escreve com fmt.Println direto
// em os.Stdout (não em cmd.OutOrStdout()), então cmd.SetOut(&buf) sozinho
// não captura as duas formas de forma comparável — capturar os.Stdout dos
// dois lados garante que a comparação é justa.
func runRootCapturing(t *testing.T, args ...string) (out string, err error) {
	t.Helper()

	cmd := NewRootCommand(testVersion())
	cmd.SetArgs(args)

	out = captureStdout(t, func() {
		err = cmd.Execute()
	})
	return out, err
}

// TestVersionFlagMatchesVersionSubcommand é o teste que trava a
// consistência central desta feature: "--version" e o subcomando "version"
// precisam imprimir EXATAMENTE o mesmo texto. O template padrão do cobra
// (defaultVersionTemplate) produziria algo como "file-manager version
// v0.1.0 (...)", diferente da saída de "file-manager version"
// ("v0.1.0 (...)") — root.SetVersionTemplate em NewRootCommand existe para
// fechar essa lacuna.
func TestVersionFlagMatchesVersionSubcommand(t *testing.T) {
	flagOut, flagErr := runRootCapturing(t, "--version")
	if flagErr != nil {
		t.Fatalf("\"--version\" devolveu erro: %v", flagErr)
	}

	subOut, subErr := runRootCapturing(t, "version")
	if subErr != nil {
		t.Fatalf("subcomando \"version\" devolveu erro: %v", subErr)
	}

	if flagOut != subOut {
		t.Fatalf("saída de \"--version\" (%q) difere da saída do subcomando \"version\" (%q)", flagOut, subOut)
	}

	want := testVersion().String() + "\n"
	if flagOut != want {
		t.Errorf("saída de \"--version\" = %q, esperava %q", flagOut, want)
	}
}

// TestVersionShorthandMatchesVersionSubcommand prova que "-v" (o atalho
// registrado manualmente em NewRootCommand, junto de "--version") produz a
// mesma saída das outras duas formas. O atalho está livre neste CLI —
// nenhuma ferramenta nem subcomando usa "-v" — confirmado por este teste
// não falhar por conflito de shorthand (o que faria cmd.Execute() devolver
// erro de parsing).
func TestVersionShorthandMatchesVersionSubcommand(t *testing.T) {
	out, err := runRootCapturing(t, "-v")
	if err != nil {
		t.Fatalf("\"-v\" devolveu erro: %v", err)
	}

	want := testVersion().String() + "\n"
	if out != want {
		t.Errorf("saída de \"-v\" = %q, esperava %q", out, want)
	}
}

// TestVersionFlagExitsSuccessfully garante que "--version" não é tratado
// como erro: cmd.Execute() precisa devolver nil, o mesmo contrato que
// app.Execute() usa para decidir o código de saída do processo (ver
// Execute, em root.go — devolve 1 só quando NewRootCommand(v).Execute()
// devolve um erro não-nil).
func TestVersionFlagExitsSuccessfully(t *testing.T) {
	_, err := runRootCapturing(t, "--version")
	if err != nil {
		t.Fatalf("\"--version\" deveria sair com sucesso (err == nil); got: %v", err)
	}
}

// TestVersionSubcommandStillWorks garante que o subcomando "version"
// continua existindo e funcionando — a flag global não pode substituí-lo
// nem removê-lo (ver comentário de newVersionCommand em root.go).
func TestVersionSubcommandStillWorks(t *testing.T) {
	out, err := runRootCapturing(t, "version")
	if err != nil {
		t.Fatalf("subcomando \"version\" devolveu erro: %v", err)
	}

	want := testVersion().String() + "\n"
	if out != want {
		t.Errorf("saída do subcomando \"version\" = %q, esperava %q", out, want)
	}
}

// TestRootHelpListsVersionFlagInPortuguese garante que "--help" continua em
// português e lista a flag "--version"/"-v" com a descrição traduzida — não
// o "version for file-manager" em inglês que o cobra geraria por padrão
// (Command.InitDefaultVersionFlag). Complementa os testes de
// usage_test.go, que cobrem a tradução dos RÓTULOS ESTRUTURAIS do template;
// este cobre especificamente o texto desta flag nova.
func TestRootHelpListsVersionFlagInPortuguese(t *testing.T) {
	out := helpOutput(t)

	if !strings.Contains(out, "-v, --version") {
		t.Errorf("saída de --help não lista a flag \"-v, --version\":\n%s", out)
	}
	if !strings.Contains(out, "mostra a versão do binário") {
		t.Errorf("saída de --help não contém a descrição em português da flag --version:\n%s", out)
	}
	if strings.Contains(out, "version for") {
		t.Errorf("saída de --help ainda contém a descrição em inglês padrão do cobra (\"version for ...\"):\n%s", out)
	}
}
