package selfupdate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseVersionValid(t *testing.T) {
	cases := map[string][3]int{
		"v1.2.3":      {1, 2, 3},
		"1.2.3":       {1, 2, 3},
		"v1.2.3-rc1":  {1, 2, 3},
		"v0.10.0":     {0, 10, 0},
		"10.20.30":    {10, 20, 30},
		"v1.2.3+meta": {1, 2, 3},
	}

	for in, want := range cases {
		got, err := ParseVersion(in)
		if err != nil {
			t.Errorf("ParseVersion(%q) devolveu erro inesperado: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseVersion(%q) = %v, esperava %v", in, got, want)
		}
	}
}

func TestParseVersionInvalid(t *testing.T) {
	cases := []string{"dev", "", "1.2", "a.b.c", "1.2.3.4", "v1..3"}

	for _, in := range cases {
		_, err := ParseVersion(in)
		if !errors.Is(err, ErrNotSemver) {
			t.Errorf("ParseVersion(%q) erro = %v, esperava ErrNotSemver", in, err)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.2.3", "v1.2.3", 0},
		{"v1.2.3", "v1.2.4", -1},
		{"v1.2.4", "v1.2.3", 1},
		{"v1.3.0", "v1.2.9", 1},
		{"v2.0.0", "v1.9.9", 1},
		{"v0.9.0", "v0.10.0", -1},
		{"v0.10.0", "v0.9.0", 1},
		{"v1.0.0", "v0.99.99", 1},
	}

	for _, tc := range cases {
		got, err := CompareVersions(tc.a, tc.b)
		if err != nil {
			t.Errorf("CompareVersions(%q, %q) devolveu erro inesperado: %v", tc.a, tc.b, err)
			continue
		}
		if got != tc.want {
			t.Errorf("CompareVersions(%q, %q) = %d, esperava %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestCompareVersionsInvalid(t *testing.T) {
	if _, err := CompareVersions("dev", "v1.0.0"); !errors.Is(err, ErrNotSemver) {
		t.Errorf("CompareVersions(dev, ...) erro = %v, esperava ErrNotSemver", err)
	}
	if _, err := CompareVersions("v1.0.0", "dev"); !errors.Is(err, ErrNotSemver) {
		t.Errorf("CompareVersions(..., dev) erro = %v, esperava ErrNotSemver", err)
	}
}

func TestAssetNameForValid(t *testing.T) {
	cases := []struct {
		goos, goarch, want string
	}{
		{"linux", "amd64", "file-manager-linux-amd64"},
		{"windows", "amd64", "file-manager-windows-amd64.exe"},
	}

	for _, tc := range cases {
		got, err := AssetNameFor(tc.goos, tc.goarch)
		if err != nil {
			t.Errorf("AssetNameFor(%q, %q) devolveu erro inesperado: %v", tc.goos, tc.goarch, err)
			continue
		}
		if got != tc.want {
			t.Errorf("AssetNameFor(%q, %q) = %q, esperava %q", tc.goos, tc.goarch, got, tc.want)
		}
	}
}

func TestAssetNameForInvalid(t *testing.T) {
	cases := []struct{ goos, goarch string }{
		{"darwin", "arm64"},
		{"linux", "386"},
	}

	for _, tc := range cases {
		_, err := AssetNameFor(tc.goos, tc.goarch)
		if err == nil {
			t.Errorf("AssetNameFor(%q, %q) esperava erro, devolveu nil", tc.goos, tc.goarch)
		}
	}
}

func TestFindAssetFound(t *testing.T) {
	r := Release{
		TagName: "v0.2.1",
		Assets: []Asset{
			{Name: "file-manager-linux-amd64", URL: "https://example.com/linux"},
			{Name: "file-manager-windows-amd64.exe", URL: "https://example.com/windows"},
		},
	}

	got, err := FindAsset(r, "file-manager-linux-amd64")
	if err != nil {
		t.Fatalf("FindAsset devolveu erro inesperado: %v", err)
	}
	if got.URL != "https://example.com/linux" {
		t.Errorf("FindAsset URL = %q, esperava %q", got.URL, "https://example.com/linux")
	}
}

func TestFindAssetNotFound(t *testing.T) {
	r := Release{TagName: "v0.2.1", Assets: []Asset{{Name: "file-manager-linux-amd64"}}}

	_, err := FindAsset(r, "file-manager-darwin-arm64")
	if !errors.Is(err, ErrNoAsset) {
		t.Errorf("FindAsset erro = %v, esperava ErrNoAsset", err)
	}
}

func TestLatestReleaseParsesJSON(t *testing.T) {
	body := `{
		"tag_name": "v0.2.1",
		"html_url": "https://github.com/SamuelGFDias/file-manager/releases/tag/v0.2.1",
		"assets": [
			{"name": "file-manager-linux-amd64", "browser_download_url": "https://example.com/linux"},
			{"name": "file-manager-windows-amd64.exe", "browser_download_url": "https://example.com/windows"}
		]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Errorf("requisição sem User-Agent")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	restore := swapAPIBaseURL(srv.URL)
	defer restore()

	r, err := LatestRelease(context.Background(), "SamuelGFDias/file-manager")
	if err != nil {
		t.Fatalf("LatestRelease devolveu erro inesperado: %v", err)
	}
	if r.TagName != "v0.2.1" {
		t.Errorf("TagName = %q, esperava %q", r.TagName, "v0.2.1")
	}
	if len(r.Assets) != 2 {
		t.Fatalf("len(Assets) = %d, esperava 2", len(r.Assets))
	}
	if r.Assets[0].Name != "file-manager-linux-amd64" {
		t.Errorf("Assets[0].Name = %q, esperava %q", r.Assets[0].Name, "file-manager-linux-amd64")
	}
}

func TestLatestReleaseNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	restore := swapAPIBaseURL(srv.URL)
	defer restore()

	_, err := LatestRelease(context.Background(), "owner/repo")
	if err == nil {
		t.Fatal("LatestRelease esperava erro, devolveu nil")
	}
	if !contains(err.Error(), "não tem nenhum release publicado") {
		t.Errorf("erro = %q, esperava mencionar ausência de releases", err.Error())
	}
}

func TestLatestReleaseForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	restore := swapAPIBaseURL(srv.URL)
	defer restore()

	_, err := LatestRelease(context.Background(), "owner/repo")
	if err == nil {
		t.Fatal("LatestRelease esperava erro, devolveu nil")
	}
	if !contains(err.Error(), "limite de requisições") {
		t.Errorf("erro = %q, esperava mencionar limite de requisições", err.Error())
	}
}

func TestDownloadHappyPath(t *testing.T) {
	const content = "conteudo-do-binario-de-teste"

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))
	defer srv.Close()

	restoreClient := swapDownloadClient(srv.Client())
	defer restoreClient()

	dest := filepath.Join(t.TempDir(), "downloaded")
	if err := Download(context.Background(), srv.URL, dest); err != nil {
		t.Fatalf("Download devolveu erro inesperado: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("erro ao ler arquivo baixado: %v", err)
	}
	if string(got) != content {
		t.Errorf("conteúdo baixado = %q, esperava %q", got, content)
	}

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("erro ao obter info do arquivo: %v", err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("arquivo baixado não tem permissão de execução: %v", info.Mode())
	}
}

func TestDownloadRejectsNonHTTPS(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "downloaded")

	err := Download(context.Background(), "http://example.com/file-manager-linux-amd64", dest)
	if err == nil {
		t.Fatal("Download esperava erro para URL http://, devolveu nil")
	}
	if !contains(err.Error(), "https") {
		t.Errorf("erro = %q, esperava mencionar https", err.Error())
	}

	if _, statErr := os.Stat(dest); statErr == nil {
		t.Error("Download criou o arquivo de destino mesmo rejeitando o esquema da URL")
	}
}

func TestReplaceAtSameDir(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "file-manager")
	next := filepath.Join(dir, "file-manager.new")

	if err := os.WriteFile(current, []byte("versao-antiga"), 0o755); err != nil {
		t.Fatalf("erro ao preparar executável atual: %v", err)
	}
	if err := os.WriteFile(next, []byte("versao-nova"), 0o755); err != nil {
		t.Fatalf("erro ao preparar executável novo: %v", err)
	}

	if err := replaceAt(current, next); err != nil {
		t.Fatalf("replaceAt devolveu erro inesperado: %v", err)
	}

	got, err := os.ReadFile(current)
	if err != nil {
		t.Fatalf("erro ao ler executável após troca: %v", err)
	}
	if string(got) != "versao-nova" {
		t.Errorf("conteúdo após replaceAt = %q, esperava %q", got, "versao-nova")
	}
}

func TestReplaceAtDifferentDir(t *testing.T) {
	currentDir := t.TempDir()
	otherDir := t.TempDir()

	current := filepath.Join(currentDir, "file-manager")
	next := filepath.Join(otherDir, "file-manager.new")

	if err := os.WriteFile(current, []byte("versao-antiga"), 0o755); err != nil {
		t.Fatalf("erro ao preparar executável atual: %v", err)
	}
	if err := os.WriteFile(next, []byte("versao-nova-em-outro-dir"), 0o755); err != nil {
		t.Fatalf("erro ao preparar executável novo: %v", err)
	}

	if err := replaceAt(current, next); err != nil {
		t.Fatalf("replaceAt devolveu erro inesperado: %v", err)
	}

	got, err := os.ReadFile(current)
	if err != nil {
		t.Fatalf("erro ao ler executável após troca: %v", err)
	}
	if string(got) != "versao-nova-em-outro-dir" {
		t.Errorf("conteúdo após replaceAt = %q, esperava %q", got, "versao-nova-em-outro-dir")
	}
}

func TestNewCheckerNonSemverNeverNotifies(t *testing.T) {
	c := NewChecker(DefaultRepo, "dev")
	c.fetch = func(ctx context.Context, repo string) (Release, error) {
		t.Fatal("fetch não deveria ser chamado para versão local não-semver")
		return Release{}, nil
	}

	c.Start()
	waitDone(t, c)

	if _, ok := c.Notice(); ok {
		t.Error("Notice() = true, esperava false para versão local não-semver")
	}
}

func TestCheckerNoticeDoesNotBlock(t *testing.T) {
	block := make(chan struct{})

	c := NewChecker(DefaultRepo, "v0.1.0")
	c.fetch = func(ctx context.Context, repo string) (Release, error) {
		<-block
		return Release{TagName: "v9.9.9"}, nil
	}

	c.Start()

	notice, ok := c.Notice()
	if ok {
		t.Errorf("Notice() devolveu pronto=true antes da checagem terminar (notice=%q)", notice)
	}

	close(block)
	waitDone(t, c)

	notice, ok = c.Notice()
	if !ok {
		t.Fatal("Notice() = false após a checagem terminar, esperava true")
	}
	if !contains(notice, "v0.1.0") || !contains(notice, "v9.9.9") {
		t.Errorf("Notice() = %q, esperava conter as duas versões", notice)
	}
}

func TestCheckerNoticeWithNewerRelease(t *testing.T) {
	body := `{"tag_name": "v0.3.0", "assets": []}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	restore := swapAPIBaseURL(srv.URL)
	defer restore()

	c := NewChecker(DefaultRepo, "v0.2.1")
	c.fetch = LatestRelease
	c.Start()
	waitDone(t, c)

	notice, ok := c.Notice()
	if !ok {
		t.Fatal("Notice() = false, esperava aviso disponível")
	}
	if !contains(notice, "v0.2.1") || !contains(notice, "v0.3.0") || !contains(notice, "update") {
		t.Errorf("Notice() = %q, esperava conter v0.2.1, v0.3.0 e \"update\"", notice)
	}
}

func TestCheckerNoticeSameVersion(t *testing.T) {
	c := NewChecker(DefaultRepo, "v0.2.1")
	c.fetch = func(ctx context.Context, repo string) (Release, error) {
		return Release{TagName: "v0.2.1"}, nil
	}

	c.Start()
	waitDone(t, c)

	if _, ok := c.Notice(); ok {
		t.Error("Notice() = true, esperava false quando já está na versão publicada")
	}
}

func TestCheckerStartOnlyFetchesOnce(t *testing.T) {
	var calls int32

	c := NewChecker(DefaultRepo, "v0.1.0")
	c.fetch = func(ctx context.Context, repo string) (Release, error) {
		atomic.AddInt32(&calls, 1)
		return Release{TagName: "v0.1.0"}, nil
	}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Start()
		}()
	}
	wg.Wait()
	waitDone(t, c)

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("fetch foi chamado %d vezes, esperava 1", got)
	}
}

func TestCheckerConcurrentNoticeCallsDuringCheck(t *testing.T) {
	block := make(chan struct{})

	c := NewChecker(DefaultRepo, "v0.1.0")
	c.fetch = func(ctx context.Context, repo string) (Release, error) {
		<-block
		return Release{TagName: "v0.2.0"}, nil
	}

	c.Start()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Notice()
		}()
	}
	close(block)
	wg.Wait()
	waitDone(t, c)
}

func TestWaitNoticeReturnsWithinTimeout(t *testing.T) {
	body := `{"tag_name": "v0.3.0", "assets": []}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	restore := swapAPIBaseURL(srv.URL)
	defer restore()

	c := NewChecker(DefaultRepo, "v0.2.1")
	c.fetch = LatestRelease
	c.Start()

	notice, ok := c.WaitNotice(5 * time.Second)
	if !ok {
		t.Fatal("WaitNotice() = false, esperava aviso disponível dentro do timeout")
	}
	if !contains(notice, "v0.2.1") || !contains(notice, "v0.3.0") {
		t.Errorf("WaitNotice() = %q, esperava conter v0.2.1 e v0.3.0", notice)
	}
}

func TestWaitNoticeTimesOutWithoutBlocking(t *testing.T) {
	block := make(chan struct{})

	c := NewChecker(DefaultRepo, "v0.1.0")
	c.fetch = func(ctx context.Context, repo string) (Release, error) {
		<-block
		return Release{TagName: "v9.9.9"}, nil
	}
	defer close(block) // libera a goroutine ao fim do teste, para não vazá-la

	c.Start()

	start := time.Now()
	notice, ok := c.WaitNotice(1 * time.Millisecond)
	elapsed := time.Since(start)

	if ok {
		t.Errorf("WaitNotice() = true (notice=%q), esperava false pois o servidor não respondeu a tempo", notice)
	}
	if notice != "" {
		t.Errorf("WaitNotice() notice = %q, esperava string vazia", notice)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("WaitNotice() levou %v para estourar timeout de 1ms; parece ter travado", elapsed)
	}
}

func TestWaitNoticeTimeoutStillNoticeableLater(t *testing.T) {
	block := make(chan struct{})

	c := NewChecker(DefaultRepo, "v0.1.0")
	c.fetch = func(ctx context.Context, repo string) (Release, error) {
		<-block
		return Release{TagName: "v9.9.9"}, nil
	}

	c.Start()

	notice, ok := c.WaitNotice(1 * time.Millisecond)
	if ok {
		t.Fatalf("WaitNotice() = true (notice=%q) antes do fetch ser liberado, esperava false", notice)
	}

	// A verificação continua em segundo plano mesmo após o timeout de
	// WaitNotice estourar: liberar o fetch agora e esperar run() terminar
	// prova que ela não foi abortada.
	close(block)
	waitDone(t, c)

	notice, ok = c.Notice()
	if !ok {
		t.Fatal("Notice() = false após a checagem terminar em segundo plano, esperava true")
	}
	if !contains(notice, "v0.1.0") || !contains(notice, "v9.9.9") {
		t.Errorf("Notice() = %q, esperava conter as duas versões", notice)
	}
}

func TestWaitNoticeReturnsImmediatelyWhenAlreadyDone(t *testing.T) {
	c := NewChecker(DefaultRepo, "v0.1.0")
	c.fetch = func(ctx context.Context, repo string) (Release, error) {
		return Release{TagName: "v0.2.0"}, nil
	}

	c.Start()
	waitDone(t, c)

	const timeout = 5 * time.Second
	start := time.Now()
	notice, ok := c.WaitNotice(timeout)
	elapsed := time.Since(start)

	if !ok {
		t.Fatal("WaitNotice() = false, esperava aviso já pronto")
	}
	if !contains(notice, "v0.1.0") || !contains(notice, "v0.2.0") {
		t.Errorf("WaitNotice() = %q, esperava conter as duas versões", notice)
	}
	if elapsed > timeout/10 {
		t.Errorf(
			"WaitNotice() levou %v com resultado já pronto; esperava retorno bem abaixo do timeout de %v",
			elapsed, timeout,
		)
	}
}

func TestWaitNoticeNonSemverReturnsImmediately(t *testing.T) {
	c := NewChecker(DefaultRepo, "dev")
	c.fetch = func(ctx context.Context, repo string) (Release, error) {
		t.Fatal("fetch não deveria ser chamado para versão local não-semver")
		return Release{}, nil
	}

	const timeout = 5 * time.Second
	start := time.Now()
	notice, ok := c.WaitNotice(timeout)
	elapsed := time.Since(start)

	if ok {
		t.Errorf("WaitNotice() = true (notice=%q), esperava false para versão local não-semver", notice)
	}
	if elapsed > timeout/10 {
		t.Errorf(
			"WaitNotice() levou %v para versão não-semver; esperava retorno imediato, bem abaixo do timeout de %v",
			elapsed, timeout,
		)
	}
}

func TestWaitNoticeAndNoticeConcurrentDuringCheck(t *testing.T) {
	block := make(chan struct{})

	c := NewChecker(DefaultRepo, "v0.1.0")
	c.fetch = func(ctx context.Context, repo string) (Release, error) {
		<-block
		return Release{TagName: "v0.2.0"}, nil
	}

	c.Start()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Notice()
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.WaitNotice(50 * time.Millisecond)
		}()
	}

	time.Sleep(10 * time.Millisecond)
	close(block)
	wg.Wait()
	waitDone(t, c)
}

// waitDone espera a checagem em segundo plano de c terminar, com um timeout
// generoso para não deixar o teste travado caso algo quebre.
func waitDone(t *testing.T, c *Checker) {
	t.Helper()
	select {
	case <-c.done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout esperando Checker.run() terminar")
	}
}

// swapAPIBaseURL substitui temporariamente apiBaseURL (usado por
// LatestRelease) por url, devolvendo uma função que restaura o valor
// original.
func swapAPIBaseURL(url string) func() {
	original := apiBaseURL
	apiBaseURL = url
	return func() { apiBaseURL = original }
}

// swapDownloadClient substitui temporariamente downloadHTTPClient (usado
// por Download) por client, devolvendo uma função que restaura o valor
// original.
func swapDownloadClient(client *http.Client) func() {
	original := downloadHTTPClient
	downloadHTTPClient = client
	return func() { downloadHTTPClient = original }
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
