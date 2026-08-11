//go:build linux

// Package testcli é um harness de teste ponta a ponta para o CLI
// file-manager: abre o binário real dentro de um pseudo-terminal (pty),
// envia teclas como um usuário faria e permite verificar o que apareceu na
// tela.
//
// Existe porque a suíte de testes de unidade do projeto, mesmo verde com
// -race, já deixou passar defeitos que só existem na costura entre peças
// corretas isoladamente — na ordem em que o usuário atravessa o menu, no
// redesenho (ou falta dele) da tela, na navegação encadeada entre
// seletores. Nenhum teste que chama funções Go diretamente exercita esse
// caminho: só abrir o processo de verdade, com um terminal de verdade, pega
// esse tipo de defeito.
//
// Este pacote não é usado pelo binário publicado — vive só em código de
// teste (a tag de build "linux", sem "e2e", é só para não pesar o build
// normal com uma dependência de x/sys/unix específica de Linux; os testes
// que de fato o utilizam ficam sob a tag "e2e", em internal/../e2e/).
package testcli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// pollInterval é o intervalo entre checagens sucessivas de Expect,
// ExpectAny e NotExpect enquanto aguardam (ou confirmam ausência de) um
// texto na tela.
const pollInterval = 20 * time.Millisecond

// ansiPattern casa sequências CSI (cores, posicionamento de cursor etc.) e
// as sequências de salvar/restaurar cursor (\x1b7, \x1b8) que o survey usa
// para redesenhar prompts no lugar. Sem removê-las, comparar texto de tela
// captada de um terminal real vira loteria: o mesmo texto visível pode vir
// cercado de sequências de escape diferentes a cada redesenho.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b[78]`)

// StripANSI remove sequências de escape ANSI/CSI de s, devolvendo só o
// texto visível. Exportada porque também é útil para quem quiser inspecionar
// um trecho de saída fora de um *Session (ex: em uma asserção customizada).
func StripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

// Options configura uma sessão iniciada por Start.
type Options struct {
	// Bin é o caminho do binário a executar. Obrigatório.
	Bin string
	// Args são os argumentos de linha de comando passados ao binário.
	Args []string
	// Dir é o diretório de trabalho do processo. Vazio usa o diretório de
	// trabalho do processo de teste.
	Dir string
	// Env são variáveis de ambiente extras (formato "CHAVE=valor"),
	// acrescentadas ao ambiente herdado do processo de teste. Usado, por
	// exemplo, para isolar perfis com XDG_CONFIG_HOME apontando para um
	// diretório temporário.
	Env []string
	// Cols é a largura do terminal virtual. Default 100 quando <= 0.
	Cols int
	// Rows é a altura do terminal virtual. Default 30 quando <= 0.
	Rows int
}

// Session é uma execução do binário real dentro de um pseudo-terminal.
type Session struct {
	t    *testing.T
	cmd  *exec.Cmd
	ptmx *os.File

	mu  sync.Mutex // protege buf
	buf bytes.Buffer

	stateMu  sync.Mutex // protege exitCode
	exitCode int

	waitDone  chan struct{}
	readDone  chan struct{}
	closeOnce sync.Once
}

// Start compila e inicia uma sessão: abre um pty, inicia opts.Bin como
// processo filho com o lado escravo do pty como stdin/stdout/stderr (e como
// terminal controlador, para que o programa detecte um TTY de verdade) e
// começa a capturar tudo que é escrito na tela. Falhas em qualquer etapa
// chamam t.Fatalf.
func Start(t *testing.T, opts Options) *Session {
	t.Helper()

	if opts.Cols <= 0 {
		opts.Cols = 100
	}
	if opts.Rows <= 0 {
		opts.Rows = 30
	}
	if opts.Bin == "" {
		t.Fatalf("testcli: Options.Bin não pode ser vazio")
	}

	ptmx, tty, err := openPty()
	if err != nil {
		t.Fatalf("testcli: erro ao abrir pty: %v", err)
	}

	ws := &unix.Winsize{Row: uint16(opts.Rows), Col: uint16(opts.Cols)}
	if err := unix.IoctlSetWinsize(int(ptmx.Fd()), unix.TIOCSWINSZ, ws); err != nil {
		_ = ptmx.Close()
		_ = tty.Close()
		t.Fatalf("testcli: erro ao definir o tamanho do terminal virtual: %v", err)
	}

	cmd := exec.Command(opts.Bin, opts.Args...)
	cmd.Dir = opts.Dir

	env := append(os.Environ(), opts.Env...)
	env = append(env, "TERM=xterm-256color")
	cmd.Env = env

	cmd.Stdin = tty
	cmd.Stdout = tty
	cmd.Stderr = tty

	// Setsid + Setctty fazem do lado escravo do pty o terminal controlador
	// do processo filho. Sem isso, isatty(stdin) falha dentro do programa e
	// ele cai no caminho não-interativo — o teste inteiro perderia o
	// sentido, já que estaríamos testando um caminho que o usuário real
	// nunca percorre com um terminal de verdade. Ctty: 0 aponta para o
	// índice 0 entre os arquivos passados (Stdin), que é o lado escravo.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    0,
	}

	if err := cmd.Start(); err != nil {
		_ = ptmx.Close()
		_ = tty.Close()
		t.Fatalf("testcli: erro ao iniciar %s: %v", opts.Bin, err)
	}

	// O processo filho já tem sua própria referência (duplicada no fork) ao
	// lado escravo; o processo de teste não precisa mais dela. Mantê-la
	// aberta aqui só atrasaria a detecção de EOF/EIO na leitura do lado
	// mestre quando o filho terminar.
	_ = tty.Close()

	s := &Session{
		t:        t,
		cmd:      cmd,
		ptmx:     ptmx,
		waitDone: make(chan struct{}),
		readDone: make(chan struct{}),
	}

	go s.readLoop()
	go s.waitLoop()

	t.Cleanup(s.Close)

	return s
}

// openPty abre um novo par mestre/escravo de pseudo-terminal via
// /dev/ptmx, seguindo o protocolo padrão do Linux: TIOCGPTN para descobrir
// o número do escravo alocado, TIOCSPTLCK para destravá-lo (sem isso
// /dev/pts/<n> não pode ser aberto) e só então abrir o escravo.
func openPty() (ptmx, tty *os.File, err error) {
	mfd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("abrir /dev/ptmx: %w", err)
	}
	ptmx = os.NewFile(uintptr(mfd), "/dev/ptmx")

	n, err := unix.IoctlGetInt(mfd, unix.TIOCGPTN)
	if err != nil {
		_ = ptmx.Close()
		return nil, nil, fmt.Errorf("ioctl TIOCGPTN: %w", err)
	}

	if err := unix.IoctlSetPointerInt(mfd, unix.TIOCSPTLCK, 0); err != nil {
		_ = ptmx.Close()
		return nil, nil, fmt.Errorf("ioctl TIOCSPTLCK: %w", err)
	}

	slaveName := fmt.Sprintf("/dev/pts/%d", n)
	sfd, err := unix.Open(slaveName, unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		_ = ptmx.Close()
		return nil, nil, fmt.Errorf("abrir %s: %w", slaveName, err)
	}
	tty = os.NewFile(uintptr(sfd), slaveName)

	return ptmx, tty, nil
}

// readLoop acumula, em s.buf, tudo que é lido do lado mestre do pty — ou
// seja, tudo que o processo filho escreveu na tela. Roda até a leitura
// falhar (o filho terminou e o lado mestre foi fechado, ou EIO — comum no
// Linux quando o último descritor do lado escravo é fechado).
func (s *Session) readLoop() {
	defer close(s.readDone)

	buf := make([]byte, 4096)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			s.mu.Lock()
			s.buf.Write(buf[:n])
			s.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// waitLoop aguarda o término do processo filho exatamente uma vez (chamar
// cmd.Wait() mais de uma vez é erro) e publica o código de saída.
func (s *Session) waitLoop() {
	err := s.cmd.Wait()

	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			code = -1
		}
	}

	s.stateMu.Lock()
	s.exitCode = code
	s.stateMu.Unlock()

	close(s.waitDone)
}

// Screen devolve tudo o que foi capturado até agora, com códigos ANSI
// removidos.
func (s *Session) Screen() string {
	s.mu.Lock()
	raw := s.buf.String()
	s.mu.Unlock()
	return StripANSI(raw)
}

// Expect aguarda até timeout que text apareça na tela. Falha o teste
// (t.Fatalf) se o texto não aparecer, anexando à mensagem toda a saída
// capturada até então — depurar um teste de terminal sem essa saída é
// adivinhação.
func (s *Session) Expect(text string, timeout time.Duration) {
	s.t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		screen := s.Screen()
		if strings.Contains(screen, text) {
			return
		}
		if time.Now().After(deadline) {
			s.t.Fatalf(
				"testcli: esperava encontrar %q na tela em até %s; não apareceu.\n--- tela capturada ---\n%s\n--- fim da tela ---",
				text, timeout, screen,
			)
			return
		}
		time.Sleep(pollInterval)
	}
}

// ExpectAny aguarda até timeout que qualquer um de texts apareça na tela e
// devolve o texto que apareceu primeiro. Falha o teste se nenhum aparecer
// dentro do prazo.
func (s *Session) ExpectAny(timeout time.Duration, texts ...string) string {
	s.t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		screen := s.Screen()
		for _, text := range texts {
			if strings.Contains(screen, text) {
				return text
			}
		}
		if time.Now().After(deadline) {
			s.t.Fatalf(
				"testcli: esperava encontrar um de %v na tela em até %s; nenhum apareceu.\n--- tela capturada ---\n%s\n--- fim da tela ---",
				texts, timeout, screen,
			)
			return ""
		}
		time.Sleep(pollInterval)
	}
}

// NotExpect falha o teste se text aparecer na tela em algum momento dentro
// de within. Usada para provar ausência (ex: a descrição de uma opção não
// selecionada não deve aparecer na tela).
func (s *Session) NotExpect(text string, within time.Duration) {
	s.t.Helper()

	deadline := time.Now().Add(within)
	for {
		screen := s.Screen()
		if strings.Contains(screen, text) {
			s.t.Fatalf(
				"testcli: não esperava encontrar %q na tela, mas apareceu.\n--- tela capturada ---\n%s\n--- fim da tela ---",
				text, screen,
			)
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(pollInterval)
	}
}

// Send envia texto cru para o processo, como se tivesse sido digitado no
// teclado.
func (s *Session) Send(text string) {
	s.t.Helper()
	if _, err := s.ptmx.Write([]byte(text)); err != nil {
		s.t.Fatalf("testcli: erro ao enviar %q: %v", text, err)
	}
}

// Down envia a seta para baixo.
func (s *Session) Down() { s.Send("\x1b[B") }

// Up envia a seta para cima.
func (s *Session) Up() { s.Send("\x1b[A") }

// Enter envia Enter (confirma o prompt atual).
func (s *Session) Enter() { s.Send("\r") }

// CtrlC envia Ctrl+C (interrompe o prompt atual / o programa).
func (s *Session) CtrlC() { s.Send("\x03") }

// Wait aguarda o processo terminar (por conta própria, ou por um Close
// concorrente) e devolve o código de saída. Falha o teste se o processo não
// terminar dentro de timeout.
func (s *Session) Wait(timeout time.Duration) int {
	s.t.Helper()

	select {
	case <-s.waitDone:
		s.stateMu.Lock()
		defer s.stateMu.Unlock()
		return s.exitCode
	case <-time.After(timeout):
		s.t.Fatalf(
			"testcli: processo não terminou em até %s.\n--- tela capturada ---\n%s\n--- fim da tela ---",
			timeout, s.Screen(),
		)
		return -1
	}
}

// Close encerra o processo (SIGTERM, e SIGKILL se não sair a tempo) e
// libera o pty, sem vazar a goroutine de leitura. Idempotente — seguro
// chamar mais de uma vez (inclusive via t.Cleanup, registrado
// automaticamente por Start).
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		if s.cmd.Process != nil {
			_ = s.cmd.Process.Signal(syscall.SIGTERM)

			select {
			case <-s.waitDone:
			case <-time.After(2 * time.Second):
				_ = s.cmd.Process.Kill()
				<-s.waitDone
			}
		}

		_ = s.ptmx.Close()
		<-s.readDone
	})
}
