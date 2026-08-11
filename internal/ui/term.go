package ui

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
)

// Clear is a function variable that clears the terminal screen.
// It can be overridden by tests for mockability.
var Clear = clearScreen

// Header is a function variable that prints the breadcrumb header.
// It can be overridden by tests for mockability.
var Header = headerScreen

// clearScreen performs a cross-platform terminal clear.
func clearScreen() {
	var cmd *exec.Cmd

	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "cls")
	} else {
		cmd = exec.Command("clear")
	}

	cmd.Stdout = os.Stdout
	err := cmd.Run()
	if err != nil {
		// Fallback to ANSI escape sequence
		fmt.Print("\033[H\033[2J")
	}
}

// headerScreen prints the breadcrumb in a highlighted format with a separator line.
func headerScreen(breadcrumb string) {
	// Print breadcrumb in cyan and bold
	headerText := color.New(color.FgCyan, color.Bold).Sprint(breadcrumb)
	fmt.Println(headerText)

	// Print a separator line
	fmt.Println(color.New(color.FgCyan).Sprint("─────────────────────────────────────────────────────────────"))

	// Print a blank line
	fmt.Println()
}

// Infof prints an info message with optional formatting arguments.
func Infof(format string, a ...any) {
	fmt.Printf(format+"\n", a...)
}

// Successf prints a success message in green with a checkmark prefix.
func Successf(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	prefix := color.New(color.FgGreen).Sprint("✓")
	fmt.Printf("%s %s\n", prefix, msg)
}

// Warnf prints a warning message in yellow with an exclamation mark prefix.
func Warnf(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	prefix := color.New(color.FgYellow).Sprint("!")
	fmt.Printf("%s %s\n", prefix, msg)
}

// Errorf prints an error message in red with an X prefix.
func Errorf(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	prefix := color.New(color.FgRed).Sprint("✗")
	fmt.Printf("%s %s\n", prefix, msg)
}

// Pause prints a prompt and waits for the user to press ENTER.
func Pause() {
	fmt.Print("Pressione ENTER para continuar...")
	reader := bufio.NewReader(os.Stdin)
	_, _ = reader.ReadString('\n')
}

// IsInteractive checks if stdin is a terminal (interactive mode).
func IsInteractive() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
}

// IsOutputTerminal informa se a saída padrão (stdout) é um terminal.
//
// Deliberadamente diferente de IsInteractive, que olha o STDIN e decide se
// o menu interativo pode abrir: aqui a pergunta é outra — "existe alguém
// olhando a saída agora, ou ela está sendo redirecionada/canalizada para
// outro programa?". Os dois sinais divergem exatamente no caso que importa:
// "file-manager --version > arquivo" mantém o stdin ligado ao terminal de
// quem digitou o comando (IsInteractive() = true), mas o stdout vai para um
// arquivo — é IsOutputTerminal() que precisa devolver false ali, para que
// scripts e redirecionamentos continuem recebendo exatamente a saída de
// sempre, sem o aviso extra de atualização disponível.
func IsOutputTerminal() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}

// WarnStderrf é a variante de Warnf que escreve na saída de erro (stderr)
// em vez de stdout. Warnf/Infof escrevem em stdout porque, historicamente,
// todo o output deste CLI (inclusive avisos do menu interativo) vive ali.
// Isso deixa de servir quando o aviso precisa conviver com uma saída que
// scripts capturam e comparam byte a byte (o valor de "--version"): mesmo
// com IsOutputTerminal() já filtrando o caso redirecionado, o aviso de
// atualização emitido ao lado da versão vai para stderr por segurança
// adicional — quem captura só stdout (ex: "versao=$(file-manager
// --version)") nunca vê o aviso misturado ao valor.
func WarnStderrf(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	prefix := color.New(color.FgYellow).Sprint("!")
	fmt.Fprintf(os.Stderr, "%s %s\n", prefix, msg)
}

// InfoStderrf é a variante de Infof que escreve em stderr, pelo mesmo
// motivo de WarnStderrf.
func InfoStderrf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
}

// Bold devolve s formatado em negrito, para destacar um trecho pontual de
// uma mensagem (ex: um nome, um valor) sem recorrer a cor.
func Bold(s string) string {
	return color.New(color.Bold).Sprint(s)
}

// Highlight devolve s em ciano e negrito, para destacar palavras-chave em
// meio a uma mensagem (nomes de etapa, opções escolhidas etc.).
func Highlight(s string) string {
	return color.New(color.FgCyan, color.Bold).Sprint(s)
}

// PathText devolve s formatado para exibir caminhos de arquivo ou pasta: cor
// magenta e sublinhado, para se destacar do texto ao redor sem se confundir
// com Highlight ou com mensagens de sucesso/aviso/erro.
func PathText(s string) string {
	return color.New(color.FgMagenta, color.Underline).Sprint(s)
}

// Count devolve "N singular" ou "N plural" (conforme n — 1 usa o singular,
// qualquer outro valor usa o plural), com o número em negrito. Ex:
// Count(12, "PDF", "PDFs") -> "12 PDFs".
func Count(n int, singular, plural string) string {
	word := plural
	if n == 1 {
		word = singular
	}
	return fmt.Sprintf("%s %s", Bold(fmt.Sprintf("%d", n)), word)
}

// Step imprime um cabeçalho de etapa destacado dentro de um fluxo mais
// longo, ex: "── Passo 2 de 4 · Pasta de destino ──". Usa amarelo e negrito
// de propósito — uma cor diferente da usada por Header (ciano) — para que
// os dois níveis de cabeçalho (tela vs. etapa dentro da tela) não se
// confundam visualmente.
func Step(n, total int, titulo string) {
	label := fmt.Sprintf("── Passo %d de %d · %s ──", n, total, titulo)
	fmt.Println(color.New(color.FgYellow, color.Bold).Sprint(label))
}

// Divider imprime uma linha divisória discreta, usada para delimitar um
// bloco de texto (ex: um resumo antes de uma confirmação importante).
func Divider() {
	fmt.Println(color.New(color.FgHiBlack).Sprint(strings.Repeat("─", 65)))
}

// Blank imprime uma linha em branco, para separar blocos de perguntas na
// tela e evitar que fiquem coladas umas nas outras.
func Blank() {
	fmt.Println()
}
