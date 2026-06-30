package daemon

import (
	"fmt"
	"path/filepath"

	"github.com/kobylinski/yucca/internal/scanner"
)

func ResolveFileCredential(projectPath, filePath, fileKey string) (string, error) {
	absPath := filepath.Join(projectPath, filePath)
	fields, err := scanner.ParseFile(absPath)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", filePath, err)
	}
	for _, f := range fields {
		if f.Key == fileKey {
			return f.Value, nil
		}
	}
	return "", fmt.Errorf("key %q not found in %s", fileKey, filePath)
}
