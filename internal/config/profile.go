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
