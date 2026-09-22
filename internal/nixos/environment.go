package nixos

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

func writeEnvironmentFiles(directory, service, shell string) error {
	files := []struct {
		name    string
		content string
	}{
		{
			name:    "environment",
			content: service,
		},
		{
			name:    "environment.sh",
			content: shell,
		},
	}

	for _, file := range files {
		path := filepath.Join(directory, file.name)
		if err := filesystem.WriteFileAtomic(path, []byte(file.content), 0o600); err != nil {
			return err
		}
	}

	return nil
}

func renderEnvironmentFiles(environment map[domain.EnvName]domain.EnvValue) (string, string, error) {
	names := make([]string, 0, len(environment))

	for name, value := range environment {
		if _, err := domain.NewEnvName(string(name)); err != nil {
			return "", "", err
		}

		if _, err := domain.NewEnvValue(string(value)); err != nil {
			return "", "", err
		}

		names = append(names, string(name))
	}

	sort.Strings(names)
	var service, shell strings.Builder

	escape := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "$", "\\$", "`", "\\`")

	for _, name := range names {
		line := name + "=\"" + escape.Replace(string(environment[domain.EnvName(name)])) + "\"\n"
		service.WriteString(line)
		shell.WriteString("export " + line)
	}

	return service.String(), shell.String(), nil
}
