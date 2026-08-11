// Package ocr fornece reconhecimento óptico de caracteres delegando ao
// executável externo "tesseract". Não é um binding CGO: o pacote apenas
// invoca o processo tesseract via os/exec, o que preserva a distribuição de
// um binário Go estático (CGO_ENABLED=0), inclusive cross-compilado de Linux
// para Windows. Em máquinas sem o tesseract instalado o binário continua
// funcionando normalmente — apenas sem a capacidade de OCR.
package ocr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ErrNotInstalled indica que o executável tesseract não foi encontrado.
var ErrNotInstalled = errors.New("ocr: tesseract não encontrado no sistema")

// Tesseract é um motor de OCR que delega ao executável tesseract.
type Tesseract struct {
	BinPath string // caminho do executável; vazio quando não encontrado

	langsOnce   sync.Once
	langsResult []string
	langsErr    error
}

// NewTesseract localiza o executável tesseract e devolve o motor.
// Sempre devolve um *Tesseract não-nil; use Available() para saber se
// funciona.
//
// A busca segue esta ordem:
//  1. Variável de ambiente TESSERACT_PATH, se definida e apontando para um
//     arquivo executável (permite indicar uma instalação fora do PATH).
//  2. exec.LookPath("tesseract").
//  3. Somente no Windows, os caminhos usuais de instalação do instalador
//     oficial, já que ele notoriamente não acrescenta o executável ao PATH.
func NewTesseract() *Tesseract {
	if p := os.Getenv("TESSERACT_PATH"); p != "" {
		if info, err := os.Stat(p); err == nil && !info.IsDir() && isExecutable(info) {
			return &Tesseract{BinPath: p}
		}
	}

	if p, err := exec.LookPath("tesseract"); err == nil {
		return &Tesseract{BinPath: p}
	}

	if runtime.GOOS == "windows" {
		candidates := []string{
			`C:\Program Files\Tesseract-OCR\tesseract.exe`,
			`C:\Program Files (x86)\Tesseract-OCR\tesseract.exe`,
		}
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			candidates = append(candidates, localAppData+`\Programs\Tesseract-OCR\tesseract.exe`)
		}
		for _, c := range candidates {
			if info, err := os.Stat(c); err == nil && !info.IsDir() {
				return &Tesseract{BinPath: c}
			}
		}
	}

	return &Tesseract{}
}

// isExecutable verifica, em sistemas com bits de permissão POSIX, se o
// arquivo é executável por alguém. No Windows a checagem de permissão de
// exec.CommandContext já cobre o caso de o arquivo não poder ser executado,
// então aqui apenas aceitamos.
func isExecutable(info os.FileInfo) bool {
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0111 != 0
}

// Available informa se o OCR pode ser usado.
func (t *Tesseract) Available() bool {
	return t.BinPath != ""
}

// ImageToText roda o OCR sobre um arquivo de imagem e devolve o texto
// reconhecido. lang vazio usa "por".
func (t *Tesseract) ImageToText(ctx context.Context, imagePath, lang string) (string, error) {
	if !t.Available() {
		return "", ErrNotInstalled
	}
	if lang == "" {
		lang = "por"
	}

	cmd := exec.CommandContext(ctx, t.BinPath, imagePath, "stdout", "-l", lang)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return "", fmt.Errorf("ocr: tesseract falhou: %w: %s", err, msg)
		}
		return "", fmt.Errorf("ocr: tesseract falhou: %w", err)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// ImageToSearchablePDF roda o OCR sobre um arquivo de imagem e grava, em
// "<outBase>.pdf", um PDF de uma página com a imagem original e uma camada
// de texto invisível sobreposta (o que o tesseract chama de configfile
// "pdf") — diferente de ImageToText, que só devolve o texto reconhecido
// como string, sem gerar arquivo nenhum. lang vazio usa "por".
//
// Medido na prática (ver AGENTS.md, decisão de ocr-pdf): ~0,9s por página, e
// o arquivo gerado costuma ficar bem maior que a imagem de origem — o
// tesseract reescreve a imagem ao montar o PDF.
func (t *Tesseract) ImageToSearchablePDF(ctx context.Context, imagePath, outBase, lang string) error {
	if !t.Available() {
		return ErrNotInstalled
	}
	if lang == "" {
		lang = "por"
	}

	cmd := exec.CommandContext(ctx, t.BinPath, imagePath, outBase, "-l", lang, "pdf")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("ocr: tesseract falhou ao gerar PDF pesquisável: %w: %s", err, msg)
		}
		return fmt.Errorf("ocr: tesseract falhou ao gerar PDF pesquisável: %w", err)
	}

	return nil
}

// Languages lista os idiomas instalados (ex: ["eng","por"]). O resultado é
// cacheado após a primeira chamada bem-sucedida ou malsucedida, evitando
// disparar um processo externo a cada consulta.
func (t *Tesseract) Languages() ([]string, error) {
	t.langsOnce.Do(func() {
		t.langsResult, t.langsErr = t.listLanguages()
	})
	return t.langsResult, t.langsErr
}

func (t *Tesseract) listLanguages() ([]string, error) {
	if !t.Available() {
		return nil, ErrNotInstalled
	}

	cmd := exec.Command(t.BinPath, "--list-langs")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return nil, fmt.Errorf("ocr: falha ao listar idiomas: %w: %s", err, msg)
		}
		return nil, fmt.Errorf("ocr: falha ao listar idiomas: %w", err)
	}

	lines := strings.Split(stdout.String(), "\n")
	if len(lines) > 0 {
		// A primeira linha é o cabeçalho:
		// `List of available languages in "..." (N):`
		lines = lines[1:]
	}

	langs := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			langs = append(langs, l)
		}
	}
	return langs, nil
}

// Version devolve a versão reportada pelo tesseract (ex: "5.5.2").
func (t *Tesseract) Version() (string, error) {
	if !t.Available() {
		return "", ErrNotInstalled
	}

	cmd := exec.Command(t.BinPath, "--version")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return "", fmt.Errorf("ocr: falha ao obter versão: %w: %s", err, msg)
		}
		return "", fmt.Errorf("ocr: falha ao obter versão: %w", err)
	}

	firstLine := strings.TrimSpace(strings.SplitN(stdout.String(), "\n", 2)[0])
	fields := strings.Fields(firstLine)
	if len(fields) >= 2 {
		return fields[1], nil
	}
	// Formato inesperado: devolve a linha inteira em vez de falhar, já que
	// essa informação é apenas informativa, não crítica.
	return firstLine, nil
}

// HasLanguage informa se um idioma está instalado.
func (t *Tesseract) HasLanguage(lang string) bool {
	langs, err := t.Languages()
	if err != nil {
		return false
	}
	for _, l := range langs {
		if l == lang {
			return true
		}
	}
	return false
}

// completionLanguageTimeout limita quanto tempo CompletionLanguages espera
// pelo processo tesseract antes de desistir e devolver a lista fixa: a
// completação de shell (Tab) nunca pode travar esperando um processo
// externo responder.
const completionLanguageTimeout = 300 * time.Millisecond

// completionLanguageNames traduz para português os códigos de idioma mais
// comuns do Tesseract, usados como descrição na completação de
// --ocr-lang. Um código sem entrada aqui ainda aparece na completação, só
// sem descrição.
var completionLanguageNames = map[string]string{
	"por": "Português",
	"eng": "Inglês",
	"spa": "Espanhol",
}

// CompletionLanguages devolve os candidatos para a completação de shell da
// flag --ocr-lang, no formato "<código>\t<descrição>" esperado por
// cobra.RegisterFlagCompletionFunc (a descrição some no bash, mas aparece
// ao lado da opção no zsh). Tenta consultar os idiomas de fato instalados
// via "tesseract --list-langs", com um limite de tempo curto
// (completionLanguageTimeout): se o tesseract não estiver instalado, ou não
// responder a tempo, devolve a lista fixa conhecida ("por", "eng") em vez
// de travar a tecla Tab do usuário. Nunca devolve erro.
func CompletionLanguages() []string {
	fixed := []string{
		"por\t" + completionLanguageNames["por"],
		"eng\t" + completionLanguageNames["eng"],
	}

	engine := NewTesseract()
	if !engine.Available() {
		return fixed
	}

	type result struct {
		langs []string
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		langs, err := engine.Languages()
		ch <- result{langs: langs, err: err}
	}()

	select {
	case r := <-ch:
		if r.err != nil || len(r.langs) == 0 {
			return fixed
		}
		out := make([]string, 0, len(r.langs))
		for _, l := range r.langs {
			if name, ok := completionLanguageNames[l]; ok {
				out = append(out, l+"\t"+name)
			} else {
				out = append(out, l)
			}
		}
		return out
	case <-time.After(completionLanguageTimeout):
		return fixed
	}
}

// InstallHint devolve uma instrução de instalação adequada ao sistema
// operacional atual, em português, para ser mostrada ao usuário final.
func InstallHint() string {
	switch runtime.GOOS {
	case "windows":
		return "Tesseract não encontrado. Baixe o instalador em " +
			"https://github.com/UB-Mannheim/tesseract/wiki e marque o idioma " +
			"Português durante a instalação. Pode ser necessário reiniciar o " +
			"terminal após instalar."
	case "linux":
		return "Tesseract não encontrado. Instale com: " +
			"sudo dnf install tesseract tesseract-langpack-por " +
			"(Fedora/RHEL) ou sudo apt install tesseract-ocr tesseract-ocr-por " +
			"(Debian/Ubuntu)."
	default:
		return "Tesseract não encontrado. Consulte as instruções de instalação " +
			"em https://tesseract-ocr.github.io/tessdoc/Installation.html"
	}
}
