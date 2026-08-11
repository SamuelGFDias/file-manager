// Package selfupdate implementa a auto-atualização do file-manager: consulta
// o último release publicado no GitHub, compara com a versão em execução e,
// quando autorizado, baixa e substitui o próprio executável.
//
// Usa somente a biblioteca padrão do Go — nenhuma dependência externa é
// necessária para este pacote.
package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultRepo é o repositório GitHub consultado pelo comando "update" quando
// nenhum outro for informado.
const DefaultRepo = "SamuelGFDias/file-manager"

// userAgent identifica as requisições deste binário junto à API do GitHub,
// que rejeita chamadas sem User-Agent.
const userAgent = "file-manager-selfupdate"

// ErrNoAsset é devolvido por FindAsset quando o release não contém um asset
// com o nome procurado.
var ErrNoAsset = errors.New("asset não encontrado no release")

// ErrNotSemver é devolvido por ParseVersion/CompareVersions quando a string
// informada não é uma versão semântica válida (ex: "dev", builds locais).
var ErrNotSemver = errors.New("versão não é um semver válido")

// apiBaseURL é o endpoint base da API do GitHub. Variável (em vez de
// constante) para poder ser substituída por um servidor de teste local nos
// testes, sem depender de rede real.
var apiBaseURL = "https://api.github.com"

// downloadHTTPClient é o cliente usado por Download. Variável para permitir
// que os testes injetem o cliente de um httptest.Server (via Client()),
// nunca desabilitando verificação de TLS em código de produção.
var downloadHTTPClient = &http.Client{Timeout: 5 * time.Minute}

// Asset representa um artefato anexado a um release do GitHub.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// Release representa o subconjunto de campos do release do GitHub usados
// pela auto-atualização.
type Release struct {
	TagName string  `json:"tag_name"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

// LatestRelease consulta o último release publicado do repositório
// "owner/repo" informado em repo.
func LatestRelease(ctx context.Context, repo string) (Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", apiBaseURL, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, fmt.Errorf("erro ao montar requisição para %s: %w", url, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)

	client := &http.Client{Timeout: 30 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("erro ao consultar o último release de %s: %w", repo, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// segue abaixo
	case http.StatusNotFound:
		return Release{}, fmt.Errorf("o repositório %q não tem nenhum release publicado", repo)
	case http.StatusForbidden:
		return Release{}, fmt.Errorf(
			"limite de requisições da API do GitHub atingido; aguarde alguns minutos e tente novamente",
		)
	default:
		return Release{}, fmt.Errorf(
			"resposta inesperada da API do GitHub ao consultar %s: status %d", repo, resp.StatusCode,
		)
	}

	var r Release
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return Release{}, fmt.Errorf("erro ao interpretar a resposta da API do GitHub: %w", err)
	}

	return r, nil
}

// Releases consulta todos os releases publicados do repositório "owner/repo"
// informado em repo, do mais recente para o mais antigo (a ordem devolvida
// pela própria API do GitHub). Rascunhos e pré-lançamentos são descartados:
// só releases de fato publicados importam para decidir se há atualização
// disponível — um rascunho não está no ar para ninguém instalar.
//
// Usada no lugar de LatestRelease pelo fluxo de verificação de atualização
// (Checker e o comando "update"), porque classificar a severidade de uma
// atualização (ClassifyUpdate) precisa enxergar todo o caminho de versões
// entre a versão em execução e a mais recente — não só a última. Continua
// sendo uma única requisição à API, então não há custo adicional de limite
// de requisições em relação a LatestRelease.
func Releases(ctx context.Context, repo string) ([]Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases?per_page=100", apiBaseURL, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("erro ao montar requisição para %s: %w", url, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)

	client := &http.Client{Timeout: 30 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro ao consultar os releases de %s: %w", repo, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// segue abaixo
	case http.StatusNotFound:
		return nil, fmt.Errorf("o repositório %q não tem nenhum release publicado", repo)
	case http.StatusForbidden:
		return nil, fmt.Errorf(
			"limite de requisições da API do GitHub atingido; aguarde alguns minutos e tente novamente",
		)
	default:
		return nil, fmt.Errorf(
			"resposta inesperada da API do GitHub ao consultar %s: status %d", repo, resp.StatusCode,
		)
	}

	// Envelope local (em vez de estender Release) para não carregar Draft e
	// Prerelease — irrelevantes fora desta função — no tipo público usado
	// pelo resto do pacote.
	var raw []struct {
		Release
		Draft      bool `json:"draft"`
		Prerelease bool `json:"prerelease"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("erro ao interpretar a resposta da API do GitHub: %w", err)
	}

	releases := make([]Release, 0, len(raw))
	for _, r := range raw {
		if r.Draft || r.Prerelease {
			continue
		}
		releases = append(releases, r.Release)
	}

	return releases, nil
}

// ParseVersion converte "v1.2.3" ou "1.2.3" (com sufixo opcional de
// pré-lançamento, ex: "-rc1") em [3]int{major, minor, patch}. Devolve
// ErrNotSemver para qualquer string que não siga esse formato — inclusive
// "dev" e strings vazias.
func ParseVersion(s string) ([3]int, error) {
	v := strings.TrimPrefix(s, "v")
	if v == "" {
		return [3]int{}, ErrNotSemver
	}

	// Descarta sufixo de pré-lançamento/build (ex: "-rc1", "+build5") sem
	// falhar — só a parte numérica major.minor.patch importa para comparar.
	if idx := strings.IndexAny(v, "-+"); idx != -1 {
		v = v[:idx]
	}

	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return [3]int{}, ErrNotSemver
	}

	var out [3]int
	for i, p := range parts {
		if p == "" {
			return [3]int{}, ErrNotSemver
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return [3]int{}, ErrNotSemver
		}
		out[i] = n
	}

	return out, nil
}

// CompareVersions devolve -1 se a < b, 0 se são iguais e 1 se a > b,
// comparando numericamente campo a campo (major, depois minor, depois
// patch) — uma comparação textual erraria, por exemplo, "v0.10.0" < "v0.9.0".
func CompareVersions(a, b string) (int, error) {
	va, err := ParseVersion(a)
	if err != nil {
		return 0, err
	}

	vb, err := ParseVersion(b)
	if err != nil {
		return 0, err
	}

	for i := range va {
		if va[i] < vb[i] {
			return -1, nil
		}
		if va[i] > vb[i] {
			return 1, nil
		}
	}

	return 0, nil
}

// Severity classifica o quanto uma atualização disponível importa para o
// usuário. Ver ClassifyUpdate para as regras completas.
type Severity int

const (
	// SeverityNone indica que não há atualização disponível — a versão em
	// execução já é a mais recente publicada (ou é mais nova que tudo).
	SeverityNone Severity = iota

	// SeverityMinor indica que só há novidade: nenhum release entre a
	// versão em execução e a mais recente corrige um defeito, e nenhum
	// muda o major. Pode esperar.
	SeverityMinor

	// SeverityPatch indica que existe correção de defeito no caminho entre
	// a versão em execução e a mais recente — mesmo que o salto pareça só
	// novidade (ex: 0.8.0 → 0.9.0), porque releases são cumulativos e a
	// correção de um patch intermediário (0.8.1) já está incluída na
	// mais recente. Continuar na versão atual significa continuar
	// exposto ao defeito já corrigido.
	SeverityPatch

	// SeverityMajor indica mudança incompatível: algum release no caminho
	// tem major maior que o da versão em execução. Tem precedência sobre
	// SeverityPatch porque uma atualização automática pode quebrar
	// automação de quem já usa o formato/flags antigos.
	SeverityMajor
)

// ClassifyUpdate decide a severidade da atualização disponível para quem
// está em current, a partir de releases (não precisa vir ordenada — a
// classificação não assume nenhuma ordem específica).
//
// current não sendo semver (ex: "dev", build local) devolve ok=false sem
// erro: build local não tem uma "versão atual" para comparar. Não existir
// nenhum release mais novo que current (já está atualizado, ou current é
// uma versão de desenvolvimento à frente de tudo que foi publicado) também
// devolve ok=false, com sev=SeverityNone.
//
// O ponto central desta função: a severidade NÃO é decidida comparando
// current só contra a versão mais recente. Ela pergunta, para TODO release
// mais novo que current — não só o mais recente — se algum tem major maior
// (SeverityMajor) ou componente de correção (patch) maior que zero
// (SeverityPatch). Isso importa porque releases são cumulativos: se current
// é 0.8.0 e a mais recente é 0.9.0, o salto por si só parece só novidade
// (0.8.0 → 0.9.0 é um bump de minor). Mas se 0.8.1 foi publicado no meio
// (0.8.0 < 0.8.1 < 0.9.0), a correção de 0.8.1 está incluída em 0.9.0 — e
// quem está em 0.8.0 está, agora mesmo, exposto ao defeito que 0.8.1
// corrigiu. Comparar só "atual" contra "mais recente" perderia esse fato;
// por isso o algoritmo varre todos os releases mais novos que current, não
// só o topo da lista.
//
// latest é sempre o release de maior versão entre os mais novos que
// current, independentemente da severidade calculada.
func ClassifyUpdate(current string, releases []Release) (latest Release, sev Severity, ok bool, err error) {
	currentVer, verErr := ParseVersion(current)
	if verErr != nil {
		return Release{}, SeverityNone, false, nil
	}

	var (
		latestVer    [3]int
		hasNewer     bool
		hasMajorBump bool
		hasPatchFix  bool
	)

	for _, r := range releases {
		v, parseErr := ParseVersion(r.TagName)
		if parseErr != nil {
			// Um release com tag fora do padrão semver não entra na
			// comparação — não há como saber se é mais novo ou mais
			// antigo que current. Defensivo: a API do GitHub não garante
			// que toda tag seja semver.
			continue
		}

		if compareVersionArrays(v, currentVer) <= 0 {
			continue // não é mais novo que current
		}

		if !hasNewer || compareVersionArrays(v, latestVer) > 0 {
			latestVer = v
			latest = r
			hasNewer = true
		}

		if v[0] > currentVer[0] {
			hasMajorBump = true
		}
		if v[2] > 0 {
			hasPatchFix = true
		}
	}

	if !hasNewer {
		return Release{}, SeverityNone, false, nil
	}

	switch {
	case hasMajorBump:
		sev = SeverityMajor
	case hasPatchFix:
		sev = SeverityPatch
	default:
		sev = SeverityMinor
	}

	return latest, sev, true, nil
}

// compareVersionArrays compara dois [3]int já parseados (major, minor,
// patch), devolvendo -1, 0 ou 1 — mesma semântica de CompareVersions, mas
// sem repetir o parsing quando os valores já estão em mãos, como dentro do
// laço de ClassifyUpdate.
func compareVersionArrays(a, b [3]int) int {
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

// NoticeText monta o texto do aviso de atualização exibido ao usuário, de
// acordo com a severidade calculada por ClassifyUpdate. Extraída como
// função pura (sem tocar em rede nem terminal) para ser testável
// isoladamente.
func NoticeText(current string, latest Release, sev Severity) string {
	switch sev {
	case SeverityPatch:
		return fmt.Sprintf(
			"correção importante disponível: %s → %s — esta atualização corrige um defeito; "+
				"continuar na versão atual pode gerar resultado inconsistente. Atualize com "+
				"\"file-manager update\"",
			current, latest.TagName,
		)
	case SeverityMajor:
		return fmt.Sprintf(
			"mudanças incompatíveis disponíveis: %s → %s — leia as notas do release antes de "+
				"atualizar: %s",
			current, latest.TagName, latest.HTMLURL,
		)
	default: // SeverityMinor (e SeverityNone, que não deveria chegar aqui)
		return fmt.Sprintf(
			"nova versão disponível: %s → %s — atualize com \"file-manager update\"",
			current, latest.TagName,
		)
	}
}

// AssetNameFor devolve o nome do artefato publicado para o sistema
// operacional/arquitetura informados. Devolve erro para qualquer
// combinação sem binário publicado.
func AssetNameFor(goos, goarch string) (string, error) {
	switch {
	case goos == "linux" && goarch == "amd64":
		return "file-manager-linux-amd64", nil
	case goos == "windows" && goarch == "amd64":
		return "file-manager-windows-amd64.exe", nil
	default:
		return "", fmt.Errorf(
			"não há binário publicado do file-manager para %s/%s; compile a partir do "+
				"código-fonte com \"go build ./cmd/file-manager\"",
			goos, goarch,
		)
	}
}

// FindAsset localiza, dentro de r, o asset cujo nome é name. Devolve
// ErrNoAsset se não existir.
func FindAsset(r Release, name string) (Asset, error) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, nil
		}
	}

	return Asset{}, fmt.Errorf("%w: %q (release %s)", ErrNoAsset, name, r.TagName)
}

// Download baixa a URL informada (que precisa ser https) para destPath, com
// permissão de execução (0o755). Só aceita esquema https: baixar e depois
// executar um binário recebido por um canal não autenticado seria um vetor
// óbvio de comprometimento.
func Download(ctx context.Context, url, destPath string) error {
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf(
			"download rejeitado: só é permitido baixar de URLs https, recebido %q", url,
		)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("erro ao montar requisição de download para %s: %w", url, err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := downloadHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("erro ao baixar %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("erro ao baixar %s: status %d", url, resp.StatusCode)
	}

	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("erro ao criar o arquivo de destino %s: %w", destPath, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("erro ao gravar o arquivo baixado em %s: %w", destPath, err)
	}

	return nil
}

// VerifyBinary executa path com o argumento "version" para confirmar que o
// binário baixado roda e não está corrompido, antes de substituir o
// executável em uso.
func VerifyBinary(ctx context.Context, path string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, path, "version").CombinedOutput()
	if err != nil {
		return fmt.Errorf(
			"o binário baixado não executou corretamente (download parece corrompido): %w", err,
		)
	}
	if strings.TrimSpace(string(out)) == "" {
		return fmt.Errorf(
			"o binário baixado não produziu nenhuma saída para \"version\" (download parece corrompido)",
		)
	}

	return nil
}

// ReplaceExecutable troca o executável em execução pelo arquivo em newPath.
func ReplaceExecutable(newPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("erro ao localizar o executável atual: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("erro ao resolver o caminho do executável atual: %w", err)
	}

	return replaceAt(resolved, newPath)
}

// replaceAt substitui o arquivo em currentPath pelo conteúdo de newPath.
// Extraída de ReplaceExecutable para ser testável sem depender do binário
// de teste em execução: os testes chamam replaceAt diretamente, apontando
// para arquivos comuns em um diretório temporário.
func replaceAt(currentPath, newPath string) error {
	dir := filepath.Dir(currentPath)

	// os.Rename falha ao cruzar sistemas de arquivos, então o arquivo novo
	// precisa estar no mesmo diretório do executável atual antes da troca.
	srcPath := newPath
	if filepath.Dir(newPath) != dir {
		tmp, err := copyToDir(newPath, dir)
		if err != nil {
			return err
		}
		defer os.Remove(tmp) // no-op se já tiver sido consumido pela troca
		srcPath = tmp
	}

	if runtime.GOOS == "windows" {
		return replaceWindows(currentPath, srcPath)
	}

	return replaceUnix(currentPath, srcPath)
}

// copyToDir copia o arquivo em src para um novo arquivo temporário dentro
// de dir, com permissão de execução, e devolve o caminho do arquivo criado.
func copyToDir(src, dir string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", fmt.Errorf("erro ao abrir %s: %w", src, err)
	}
	defer in.Close()

	tmp, err := os.CreateTemp(dir, ".file-manager-update-*")
	if err != nil {
		return "", fmt.Errorf("erro ao criar arquivo temporário em %s: %w", dir, err)
	}
	tmpPath := tmp.Name()

	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("erro ao copiar %s para %s: %w", src, dir, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("erro ao finalizar a cópia em %s: %w", tmpPath, err)
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("erro ao ajustar a permissão de %s: %w", tmpPath, err)
	}

	return tmpPath, nil
}

// replaceUnix troca currentPath por srcPath em Linux/macOS. Não é possível
// escrever por cima de um executável em uso (ETXTBSY), mas os.Rename por
// cima funciona: o processo em execução continua servindo o inode antigo
// até terminar.
func replaceUnix(currentPath, srcPath string) error {
	if err := os.Rename(srcPath, currentPath); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf(
				"sem permissão para escrever em %s; rode como root/sudo ou mova o file-manager "+
					"para uma pasta do seu usuário: %w",
				currentPath, err,
			)
		}
		return fmt.Errorf("erro ao substituir o executável em %s: %w", currentPath, err)
	}

	return nil
}

// replaceWindows troca currentPath por srcPath no Windows. Não é possível
// apagar nem sobrescrever um .exe em execução, mas é possível renomeá-lo:
// renomeia o executável atual para "<nome>.old", move o novo para o lugar
// dele e tenta apagar o .old (ignorando erro — o Windows só libera o
// arquivo depois que o processo antigo terminar; o .old pode ficar para
// trás e é seguro apagá-lo manualmente). Se mover o novo falhar, restaura o
// .old para o nome original — deixar o usuário sem executável nenhum é o
// pior resultado possível.
func replaceWindows(currentPath, srcPath string) error {
	oldPath := currentPath + ".old"
	_ = os.Remove(oldPath) // sobra de uma atualização anterior, se houver

	if err := os.Rename(currentPath, oldPath); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf(
				"sem permissão para escrever em %s; rode como administrador ou mova o "+
					"file-manager para uma pasta do seu usuário: %w",
				currentPath, err,
			)
		}
		return fmt.Errorf("erro ao renomear o executável atual em %s: %w", currentPath, err)
	}

	if err := os.Rename(srcPath, currentPath); err != nil {
		if restoreErr := os.Rename(oldPath, currentPath); restoreErr != nil {
			return fmt.Errorf(
				"erro ao mover o novo executável para %s (%v), e falha ao restaurar a versão "+
					"anterior (%v); o executável original está preservado em %s",
				currentPath, err, restoreErr, oldPath,
			)
		}
		return fmt.Errorf("erro ao mover o novo executável para %s: %w", currentPath, err)
	}

	// Melhor esforço: pode falhar enquanto o processo antigo não terminar.
	_ = os.Remove(oldPath)

	return nil
}

// Checker verifica em segundo plano, uma única vez, se há uma versão mais
// recente publicada do que a versão em execução. Pensado para ser criado no
// arranque do menu interativo: Start dispara a checagem em goroutine e
// Notice consulta o resultado sem nunca bloquear, para não atrasar a
// abertura do menu nem repetir a consulta a cada redesenho da tela.
type Checker struct {
	repo           string
	currentVersion string
	timeout        time.Duration
	fetch          func(ctx context.Context, repo string) ([]Release, error)

	once sync.Once
	done chan struct{}

	mu       sync.Mutex
	notice   string
	severity Severity
	ready    bool
}

// NewChecker cria o verificador para currentVersion (a versão em execução)
// contra o repositório repo.
func NewChecker(repo, currentVersion string) *Checker {
	return &Checker{
		repo:           repo,
		currentVersion: currentVersion,
		timeout:        5 * time.Second,
		fetch:          Releases,
		done:           make(chan struct{}),
	}
}

// Start dispara a verificação uma única vez, em goroutine. Chamadas
// repetidas são no-op: não disparam nova verificação nem nova requisição à
// API do GitHub.
func (c *Checker) Start() {
	c.once.Do(func() {
		go c.run()
	})
}

// run executa a checagem. Qualquer falha (rede, versão local não-semver,
// já atualizado) termina em silêncio, sem preencher o aviso — o usuário não
// veio até aqui para depurar conectividade.
func (c *Checker) run() {
	defer close(c.done)

	if _, err := ParseVersion(c.currentVersion); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	releases, err := c.fetch(ctx, c.repo)
	if err != nil {
		return
	}

	latest, sev, ok, err := ClassifyUpdate(c.currentVersion, releases)
	if err != nil || !ok {
		return
	}

	c.mu.Lock()
	c.notice = NoticeText(c.currentVersion, latest, sev)
	c.severity = sev
	c.ready = true
	c.mu.Unlock()
}

// Notice devolve o texto do aviso pronto para exibição, e false quando não
// há aviso a mostrar (ainda verificando, sem rede, já atualizado, ou versão
// local não-semver). Nunca bloqueia.
func (c *Checker) Notice() (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.notice, c.ready
}

// Severity devolve a severidade do aviso pronto — só faz sentido consultar
// depois que Notice() ou WaitNotice() devolveram ok=true; antes disso
// devolve SeverityNone. Método separado (em vez de mudar a assinatura de
// Notice/WaitNotice) para não quebrar quem só quer o texto: o menu
// principal usa Severity() só para decidir entre ui.Warnf (correção,
// incompatibilidade) e ui.Infof (novidade).
func (c *Checker) Severity() Severity {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.severity
}

// WaitNotice aguarda no máximo timeout pelo resultado da verificação e
// devolve o aviso. Se o resultado não chegar a tempo, devolve ("", false) e
// a verificação continua em segundo plano — uma consulta posterior via
// Notice() pode devolvê-lo.
//
// Pensado para a primeira renderização do menu: a checagem normalmente
// termina bem antes do timeout (a chamada à API do GitHub leva ~250ms em
// condições normais), então WaitNotice quase sempre devolve o aviso a
// tempo; sem rede (ou com rede lenta), o timeout garante que o pior caso é
// limitado e nunca trava a abertura do menu indefinidamente.
//
// Versão local não-semver (ex: "dev") devolve ("", false) imediatamente,
// sem esperar nada — build local não deve pagar nenhuma latência aqui,
// mesmo que o timeout informado seja alto.
//
// Se Start() nunca foi chamado, WaitNotice não trava: devolve ("", false)
// somente ao fim do timeout, já que não há como distinguir "verificação
// nunca vai rodar" de "verificação ainda não rodou" sem essa espera.
func (c *Checker) WaitNotice(timeout time.Duration) (string, bool) {
	if _, err := ParseVersion(c.currentVersion); err != nil {
		return "", false
	}

	select {
	case <-c.done:
		return c.Notice()
	case <-time.After(timeout):
		return "", false
	}
}
