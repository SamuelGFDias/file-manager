package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Profile é o envelope completo persistido em disco para um perfil.
type Profile struct {
	Name      string    `yaml:"name"`
	Tool      string    `yaml:"tool"`
	CreatedAt time.Time `yaml:"created_at"`
	UpdatedAt time.Time `yaml:"updated_at"`
	Data      yaml.Node `yaml:"data"`
}

// List retorna os nomes (sem a extensão .yaml) dos perfis existentes para
// uma ferramenta, ordenados alfabeticamente. Retorna um slice vazio (não um
// erro) quando o diretório de perfis ainda não existe.
func List(toolID string) ([]string, error) {
	dir, err := ProfilesDir(toolID)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		names = append(names, strings.TrimSuffix(entry.Name(), ".yaml"))
	}

	sort.Strings(names)
	return names, nil
}

// Exists indica se um perfil com o nome informado já existe.
func Exists(toolID, name string) (bool, error) {
	if err := ValidateName(name); err != nil {
		return false, err
	}

	path, err := ProfilePath(toolID, name)
	if err != nil {
		return false, err
	}

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// LoadProfile carrega o envelope completo do perfil, sem decodificar o
// campo Data em uma struct específica.
func LoadProfile(toolID, name string) (*Profile, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}

	path, err := ProfilePath(toolID, name)
	if err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("perfil %q não encontrado: %w", name, err)
		}
		return nil, err
	}

	var profile Profile
	if err := yaml.Unmarshal(raw, &profile); err != nil {
		return nil, fmt.Errorf("erro ao decodificar perfil %q: %w", name, err)
	}

	return &profile, nil
}

// Load carrega um perfil e decodifica o campo `data` do YAML em out.
func Load(toolID, name string, out any) error {
	if err := ValidateName(name); err != nil {
		return err
	}

	profile, err := LoadProfile(toolID, name)
	if err != nil {
		return err
	}

	if err := profile.Data.Decode(out); err != nil {
		return fmt.Errorf("erro ao decodificar dados do perfil %q: %w", name, err)
	}

	return nil
}

// Save grava o envelope completo do perfil em disco. Se um perfil com o
// mesmo nome já existir, o campo CreatedAt é preservado; caso contrário, é
// definido como o momento atual. UpdatedAt é sempre atualizado para o
// momento atual.
func Save(toolID, name string, data any) error {
	if err := ValidateName(name); err != nil {
		return err
	}

	path, err := ProfilePath(toolID, name)
	if err != nil {
		return err
	}

	now := time.Now()
	createdAt := now

	if existing, err := LoadProfile(toolID, name); err == nil {
		createdAt = existing.CreatedAt
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	var dataNode yaml.Node
	if err := dataNode.Encode(data); err != nil {
		return fmt.Errorf("erro ao codificar dados do perfil %q: %w", name, err)
	}

	profile := Profile{
		Name:      name,
		Tool:      toolID,
		CreatedAt: createdAt,
		UpdatedAt: now,
		Data:      dataNode,
	}

	out, err := yaml.Marshal(&profile)
	if err != nil {
		return fmt.Errorf("erro ao codificar perfil %q: %w", name, err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(path, out, 0o644); err != nil {
		return err
	}

	return nil
}

// Delete remove o arquivo de perfil. Retorna um erro detectável com
// errors.Is(err, os.ErrNotExist) se o perfil não existir.
func Delete(toolID, name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}

	path, err := ProfilePath(toolID, name)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("perfil %q não encontrado: %w", name, err)
		}
		return err
	}

	return nil
}

// ExportProfile grava o perfil identificado por toolID/name num arquivo
// externo em destPath, preservando exatamente o mesmo envelope usado
// internamente (name, tool, created_at, updated_at, data). Isso torna
// ExportProfile e ImportProfile simétricos: o arquivo exportado é lido de
// volta por ReadProfileFile sem nenhuma tradução de formato. Cria o
// diretório de destino se ele ainda não existir.
func ExportProfile(toolID, name, destPath string) error {
	profile, err := LoadProfile(toolID, name)
	if err != nil {
		return err
	}

	out, err := yaml.Marshal(profile)
	if err != nil {
		return fmt.Errorf("erro ao codificar perfil %q: %w", name, err)
	}

	if dir := filepath.Dir(destPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("erro ao criar diretório de destino %q: %w", dir, err)
		}
	}

	if err := os.WriteFile(destPath, out, 0o644); err != nil {
		return fmt.Errorf("erro ao gravar arquivo de exportação %q: %w", destPath, err)
	}

	return nil
}

// ImportedProfile descreve o que foi lido de um arquivo de perfil externo:
// o nome e a ferramenta declarados no arquivo, e o conteúdo de "data" ainda
// não decodificado (quem chama ReadProfileFile decide contra qual struct de
// Options decodificar, já que só o registro de ferramentas conhece isso).
type ImportedProfile struct {
	Name string
	Tool string
	Node yaml.Node
}

// DecodeError envolve o erro devolvido por ImportedProfile.Node.Decode()
// numa mensagem compreensível para quem não é desenvolvedor: quem recebe um
// arquivo de perfil por e-mail e tenta importar é exatamente a pessoa que
// não faz ideia do que significa um erro cru do decodificador de YAML
// (texto em inglês, citando um tipo interno do Go como
// "[]organizepdf.LevelSpec"). A mensagem principal nomeia a ferramenta e o
// arquivo, explica as causas mais comuns — arquivo corrompido, editado à
// mão de forma incorreta, ou gerado por uma versão diferente do
// file-manager — e sugere pedir um novo arquivo a quem enviou. O erro
// original continua acessível a quem for depurar via errors.Unwrap/Is
// (encapsulado com %w) e aparece numa segunda linha, prefixada "detalhe
// técnico:", para não sumir da mensagem.
func DecodeError(toolID, path string, err error) error {
	return fmt.Errorf(
		"o conteúdo do perfil no arquivo %q não é compatível com a ferramenta %q. Isso costuma "+
			"acontecer quando o arquivo está corrompido, foi editado à mão de forma incorreta, ou "+
			"foi exportado por uma versão diferente do file-manager. Peça a quem enviou o arquivo "+
			"para exportá-lo novamente.\ndetalhe técnico: %w",
		path, toolID, err,
	)
}

// ReadProfileFile lê e valida a estrutura de um arquivo de perfil externo:
// o arquivo precisa existir e ser um YAML de perfil válido, com os campos
// "tool" e "name" preenchidos e "name" aprovado por ValidateName. As
// mensagens de erro nomeiam o campo problemático, pensadas para quem recebe
// o arquivo por e-mail ou mensagem e precisa entender o que veio errado sem
// olhar o código.
func ReadProfileFile(path string) (ImportedProfile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ImportedProfile{}, fmt.Errorf("arquivo de perfil %q não encontrado: %w", path, err)
		}
		return ImportedProfile{}, fmt.Errorf("erro ao ler arquivo de perfil %q: %w", path, err)
	}

	var profile Profile
	if err := yaml.Unmarshal(raw, &profile); err != nil {
		return ImportedProfile{}, fmt.Errorf("arquivo %q não é um YAML de perfil válido: %w", path, err)
	}

	if strings.TrimSpace(profile.Tool) == "" {
		return ImportedProfile{}, fmt.Errorf("arquivo %q inválido: campo \"tool\" está vazio", path)
	}

	if strings.TrimSpace(profile.Name) == "" {
		return ImportedProfile{}, fmt.Errorf("arquivo %q inválido: campo \"name\" está vazio", path)
	}

	if err := ValidateName(profile.Name); err != nil {
		return ImportedProfile{}, fmt.Errorf("arquivo %q inválido: campo \"name\" (%q) é inválido: %w", path, profile.Name, err)
	}

	return ImportedProfile{
		Name: profile.Name,
		Tool: profile.Tool,
		Node: profile.Data,
	}, nil
}

// ImportProfile grava um perfil lido de arquivo (via ReadProfileFile) no
// diretório de configuração, sob o nome de destino informado (que pode ser
// diferente do nome original do arquivo). Valida o nome de destino com
// ValidateName. Se já existir um perfil com esse nome para p.Tool,
// overwrite=false devolve erro sem gravar nada; overwrite=true sobrescreve
// e preserva o created_at do perfil existente.
func ImportProfile(p ImportedProfile, name string, overwrite bool) error {
	if err := ValidateName(name); err != nil {
		return err
	}

	path, err := ProfilePath(p.Tool, name)
	if err != nil {
		return err
	}

	now := time.Now()
	createdAt := now

	existing, err := LoadProfile(p.Tool, name)
	switch {
	case err == nil:
		if !overwrite {
			return fmt.Errorf(
				"já existe um perfil chamado %q para a ferramenta %q; use a opção de sobrescrever para importar mesmo assim",
				name, p.Tool,
			)
		}
		createdAt = existing.CreatedAt
	case errors.Is(err, os.ErrNotExist):
		// Perfil ainda não existe: segue com createdAt = now, definido acima.
	default:
		return err
	}

	profile := Profile{
		Name:      name,
		Tool:      p.Tool,
		CreatedAt: createdAt,
		UpdatedAt: now,
		Data:      p.Node,
	}

	out, err := yaml.Marshal(&profile)
	if err != nil {
		return fmt.Errorf("erro ao codificar perfil %q: %w", name, err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(path, out, 0o644); err != nil {
		return err
	}

	return nil
}
