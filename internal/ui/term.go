package ui

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"

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
