package app

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SamuelGFDias/file-manager/internal/selfupdate"
)

// runRootCapturing executa o comando raiz com args, capturando SÓ o que foi
// escrito em os.Stdout (via captureStdout, definido em undo_test.go).
// Necessário porque o subcomando "version" escreve com fmt.Println direto
// em os.Stdout (não em cmd.OutOrStdout()), então cmd.SetOut(&buf) sozinho
// não captura as duas formas de forma comparável — capturar os.Stdout dos
// dois lados garante que a comparação é justa.
//
// Deliberadamente não captura stderr: o aviso de atualização disponível
// (quando a saída é um terminal — não é o caso aqui, já que os.Pipe não é
// um tty, então nenhuma consulta de rede acontece nestes testes) vive em
// stderr (ui.WarnStderrf/ui.InfoStderrf), justamente para não contaminar o
// stdout que esta função compara. A garantia central desta suíte —
// "--version", "-v" e "version" produzem a MESMA saída — vale para stdout;
// ver TestWarnStderrfAndInfoStderrfWriteToStderr em
// internal/ui/term_test.go para a prova de que essas duas funções (as
// únicas usadas para imprimir o aviso) nunca escrevem em stdout, e os
// testes abaixo (TestPrintVersionNonTerminalNeverConsultsNetwork e
// TestPrintVersionOutputStreams) para a prova, no nível de printVersion,
// de que nada é escrito em stderr quando não há aviso a mostrar.
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

// captureStdoutAndStderr redireciona tanto os.Stdout quanto os.Stderr
// durante fn e devolve o que foi escrito em cada um separadamente. Variante
// de captureStdout (undo_test.go) que também cobre stderr — necessária
// aqui porque printVersion escreve o aviso de atualização em stderr
// (ui.WarnStderrf/ui.InfoStderrf), e os testes deste arquivo precisam
// provar tanto o que vai para stdout quanto a ausência de vazamento para lá
// vindo de stderr (e vice-versa).
func captureStdoutAndStderr(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() (stdout): %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() (stderr): %v", err)
	}

	originalOut, originalErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	defer func() { os.Stdout, os.Stderr = originalOut, originalErr }()

	fn()

	if err := outW.Close(); err != nil {
		t.Fatalf("fechar pipe de stdout: %v", err)
	}
	if err := errW.Close(); err != nil {
		t.Fatalf("fechar pipe de stderr: %v", err)
	}

	var outBuf, errBuf bytes.Buffer
	if _, err := io.Copy(&outBuf, outR); err != nil {
		t.Fatalf("ler pipe de stdout: %v", err)
	}
	if _, err := io.Copy(&errBuf, errR); err != nil {
		t.Fatalf("ler pipe de stderr: %v", err)
	}

	return outBuf.String(), errBuf.String()
}

// countingChecker devolve uma factory de *selfupdate.Checker (a mesma
// assinatura do parâmetro newChecker de printVersion) que conta quantas
// vezes foi chamada, delegando para selfupdate.NewChecker de verdade a cada
// chamada. Usado para provar, sem precisar simular uma resposta HTTP
// completa, que printVersion nem chega a CRIAR um Checker (e, portanto,
// nunca chama Start(), que é o que dispara a consulta de rede em
// segundo plano) quando outputIsTerminal é false.
func countingChecker() (factory func(repo, currentVersion string) *selfupdate.Checker, calls *int) {
	n := 0
	return func(repo, currentVersion string) *selfupdate.Checker {
		n++
		return selfupdate.NewChecker(repo, currentVersion)
	}, &n
}

// TestPrintVersionNonTerminalNeverConsultsNetwork é o teste que protege
// scripts: com outputIsTerminal=false (o caso de "file-manager --version >
// arquivo" ou de qualquer pipe), printVersion precisa imprimir só a versão
// e devolver sem sequer construir um selfupdate.Checker — se construísse,
// já teria disparado Start() e, com ele, a consulta de rede que este
// desenho existe para evitar no caminho não-interativo. A contagem de
// chamadas da factory (não uma checagem de "nenhuma requisição HTTP
// aconteceu", que exigiria simular a rede) é a prova: se
// TestPrintVersionNonTerminalNeverConsultsNetwork falhar porque calls > 0,
// é sinal de que alguém moveu a checagem de outputIsTerminal para depois
// da criação do Checker — regressão que reabriria a lentidão/dependência
// de rede que a Armadilha 1 do desenho original proíbe.
func TestPrintVersionNonTerminalNeverConsultsNetwork(t *testing.T) {
	fakeNewChecker, calls := countingChecker()

	stdout, stderr := captureStdoutAndStderr(t, func() {
		printVersion(testVersion(), false, fakeNewChecker)
	})

	if *calls != 0 {
		t.Errorf("newChecker foi chamado %d vez(es) com outputIsTerminal=false, esperava 0 (nenhuma consulta de rede)", *calls)
	}

	want := testVersion().String() + "\n"
	if stdout != want {
		t.Errorf("stdout = %q, esperava %q", stdout, want)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, esperava vazio (sem terminal, não pode haver aviso)", stderr)
	}
}

// TestPrintVersionNonSemverNeverWarns cobre compilação local (versão "dev",
// não-semver): mesmo com outputIsTerminal=true, não pode haver aviso.
// selfupdate.Checker.WaitNotice devolve imediatamente (sem tocar rede nem
// esperar o timeout) quando a versão corrente não é semver válido — este
// teste prova o reflexo disso em printVersion: só a versão sai em stdout,
// stderr fica vazio, e o teste não fica preso pelo timeout de 1s (prova
// indireta de que nenhuma espera de rede aconteceu).
func TestPrintVersionNonSemverNeverWarns(t *testing.T) {
	devVersion := Version{Version: "dev", Commit: "none", Date: "unknown"}
	fakeNewChecker, calls := countingChecker()

	start := time.Now()
	stdout, stderr := captureStdoutAndStderr(t, func() {
		printVersion(devVersion, true, fakeNewChecker)
	})
	elapsed := time.Since(start)

	if *calls != 1 {
		t.Errorf("newChecker foi chamado %d vez(es), esperava 1 (o caminho de terminal chega a criar o Checker)", *calls)
	}

	want := devVersion.String() + "\n"
	if stdout != want {
		t.Errorf("stdout = %q, esperava %q", stdout, want)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, esperava vazio (versão local não é comparável, não há aviso)", stderr)
	}
	if elapsed >= versionNoticeTimeout {
		t.Errorf(
			"printVersion levou %s para versão não-semver, esperava retorno praticamente imediato (WaitNotice não deveria pagar o timeout de %s)",
			elapsed, versionNoticeTimeout,
		)
	}
}

// TestPrintVersionLineIsAlwaysFirst prova, em dois cenários que não tocam
// rede (não-terminal, e terminal com versão local "dev"), que a linha da
// versão é sempre a PRIMEIRA linha de stdout — e, no desenho atual, a
// ÚNICA, já que o aviso (quando existe) vai inteiramente para stderr. Isso
// importa para quem faz "file-manager --version | head -1" ou equivalente.
func TestPrintVersionLineIsAlwaysFirst(t *testing.T) {
	scenarios := []struct {
		name             string
		outputIsTerminal bool
		version          Version
	}{
		{"nao-terminal", false, testVersion()},
		{"terminal-versao-local-dev", true, Version{Version: "dev", Commit: "none", Date: "unknown"}},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			fakeNewChecker, _ := countingChecker()

			stdout, _ := captureStdoutAndStderr(t, func() {
				printVersion(sc.version, sc.outputIsTerminal, fakeNewChecker)
			})

			lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
			if len(lines) == 0 || lines[0] != sc.version.String() {
				t.Fatalf("primeira linha de stdout = %q, esperava %q\nstdout completo: %q", lines[0], sc.version.String(), stdout)
			}
		})
	}
}

// TestPrintVersionOutputStreams documenta, num único teste, para onde cada
// coisa vai: a versão sempre em stdout, nunca em stderr; e — nos cenários
// sem rede cobertos pelos testes acima — nenhum aviso em nenhum dos dois,
// já que não há o que avisar. Funciona como uma rede de segurança contra um
// refactor que, por engano, troque fmt.Println(v.String()) por uma chamada
// que escreva em stderr, ou que mova o aviso de volta para stdout.
func TestPrintVersionOutputStreams(t *testing.T) {
	fakeNewChecker, _ := countingChecker()

	stdout, stderr := captureStdoutAndStderr(t, func() {
		printVersion(testVersion(), false, fakeNewChecker)
	})

	if !strings.Contains(stdout, testVersion().String()) {
		t.Errorf("stdout não contém a versão: %q", stdout)
	}
	if strings.Contains(stderr, testVersion().String()) {
		t.Errorf("stderr não deveria conter a versão: %q", stderr)
	}
}
