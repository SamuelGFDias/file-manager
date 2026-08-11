package ocr

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- Testes que NÃO dependem do binário tesseract instalado ---

func TestNewTesseractNeverNil(t *testing.T) {
	tess := NewTesseract()
	if tess == nil {
		t.Fatal("NewTesseract() não deve devolver nil")
	}
}

func TestAvailableFalseWhenBinPathEmpty(t *testing.T) {
	tess := &Tesseract{}
	if tess.Available() {
		t.Error("Available() deveria ser false quando BinPath está vazio")
	}
}

func TestImageToTextErrNotInstalled(t *testing.T) {
	tess := &Tesseract{}
	_, err := tess.ImageToText(context.Background(), "qualquer.png", "por")
	if err == nil {
		t.Fatal("esperava erro quando BinPath está vazio")
	}
	if !errors.Is(err, ErrNotInstalled) {
		t.Errorf("esperava errors.Is(err, ErrNotInstalled), obtive: %v", err)
	}
}

func TestHasLanguageFalseWhenUnavailable(t *testing.T) {
	tess := &Tesseract{}
	if tess.HasLanguage("por") {
		t.Error("HasLanguage() deveria ser false quando o motor está indisponível")
	}
}

func TestInstallHintNotEmpty(t *testing.T) {
	hint := InstallHint()
	if strings.TrimSpace(hint) == "" {
		t.Error("InstallHint() não deveria devolver string vazia")
	}
}

// --- Testes que dependem do binário tesseract instalado ---

func TestVersion(t *testing.T) {
	tess := NewTesseract()
	if !tess.Available() {
		t.Skip("tesseract não instalado")
	}

	version, err := tess.Version()
	if err != nil {
		t.Fatalf("Version() falhou: %v", err)
	}
	if strings.TrimSpace(version) == "" {
		t.Error("Version() devolveu string vazia")
	}
}

func TestLanguages(t *testing.T) {
	tess := NewTesseract()
	if !tess.Available() {
		t.Skip("tesseract não instalado")
	}

	langs, err := tess.Languages()
	if err != nil {
		t.Fatalf("Languages() falhou: %v", err)
	}
	if len(langs) == 0 {
		t.Fatal("Languages() devolveu slice vazio")
	}
	for _, l := range langs {
		if strings.Contains(l, "List of available languages") {
			t.Errorf("Languages() não deveria conter a linha de cabeçalho, obtive: %q", l)
		}
	}
}

func TestHasLanguage(t *testing.T) {
	tess := NewTesseract()
	if !tess.Available() {
		t.Skip("tesseract não instalado")
	}

	if !tess.HasLanguage("por") {
		t.Error(`HasLanguage("por") deveria ser true`)
	}
	if tess.HasLanguage("xyz_inexistente") {
		t.Error(`HasLanguage("xyz_inexistente") deveria ser false`)
	}
}

// TestImageToText roda o OCR sobre uma imagem PNG gerada em tempo de teste.
//
// A imagem contém apenas formas geométricas simples (retângulos pretos sobre
// fundo branco), não texto tipográfico real: desenhar glifos reconhecíveis
// pelo tesseract exigiria uma biblioteca de renderização de fonte, que está
// fora do escopo deste pacote (stdlib apenas). Por isso a asserção se limita
// a verificar que ImageToText não devolve erro ao processar uma imagem
// válida — o conteúdo reconhecido pode legitimamente vir vazio, e afirmar
// um texto específico aqui seria um teste frágil.
func TestImageToText(t *testing.T) {
	tess := NewTesseract()
	if !tess.Available() {
		t.Skip("tesseract não instalado")
	}

	imgPath := generateTestImage(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := tess.ImageToText(ctx, imgPath, "por")
	if err != nil {
		t.Fatalf("ImageToText() falhou sobre uma imagem válida: %v", err)
	}
}

func TestImageToTextCanceledContext(t *testing.T) {
	tess := NewTesseract()
	if !tess.Available() {
		t.Skip("tesseract não instalado")
	}

	imgPath := generateTestImage(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // já cancelado antes de rodar

	_, err := tess.ImageToText(ctx, imgPath, "por")
	if err == nil {
		t.Fatal("esperava erro com contexto já cancelado")
	}
}

// generateTestImage cria um PNG simples (fundo branco com um retângulo
// preto) em um diretório temporário e devolve seu caminho.
func generateTestImage(t *testing.T) string {
	t.Helper()

	width, height := 200, 80
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	black := color.RGBA{R: 0, G: 0, B: 0, A: 255}

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, white)
		}
	}
	// Um retângulo preto simples ao centro, apenas para gerar conteúdo não
	// trivial na imagem.
	for y := 30; y < 50; y++ {
		for x := 40; x < 160; x++ {
			img.Set(x, y, black)
		}
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test.png")

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("criar arquivo de imagem: %v", err)
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		t.Fatalf("codificar PNG: %v", err)
	}

	return path
}
