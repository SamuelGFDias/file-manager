package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// AppName é o nome da aplicação, usado como subdiretório dentro do diretório
// de configuração do usuário.
const AppName = "file-manager"

// userConfigDir resolve o diretório de configuração do usuário. É uma
// variável (e não uma chamada direta a os.UserConfigDir) para permitir que
// os testes a substituam por um diretório temporário.
var userConfigDir = os.UserConfigDir

// validNamePattern define os caracteres permitidos em um nome de perfil.
var validNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ValidateName valida um nome de perfil, garantindo que ele não possa ser
// usado para escapar do diretório de perfis (path traversal) e que contenha
// apenas caracteres seguros.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("nome de perfil não pode ser vazio")
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("nome de perfil não pode conter separador de caminho: %q", name)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("nome de perfil não pode conter \"..\": %q", name)
	}
	if !validNamePattern.MatchString(name) {
		return fmt.Errorf("nome de perfil contém caracteres inválidos (permitido: letras, números, '.', '_', '-'): %q", name)
	}
	return nil
}

// BaseDir retorna o diretório base de configuração da aplicação, algo como
// <os.UserConfigDir()>/file-manager.
func BaseDir() (string, error) {
	dir, err := userConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, AppName), nil
}

// ProfilesDir retorna o diretório onde os perfis de uma ferramenta
// específica (toolID) são armazenados.
func ProfilesDir(toolID string) (string, error) {
	base, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "profiles", toolID), nil
}

// ProfilePath retorna o caminho completo do arquivo YAML de um perfil.
func ProfilePath(toolID, name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	dir, err := ProfilesDir(toolID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".yaml"), nil
}
