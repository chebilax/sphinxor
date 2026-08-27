package cerbos

import (
	"fmt"
	"os"
	"path/filepath"
)

// WritePolicies writes one Cerbos resource policy YAML file per resource
// in result into dir (created if needed), named "<resource>.yaml".
// Returns the paths written, in the same order as ResourceNames.
func WritePolicies(dir string, result Result) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating %s: %w", dir, err)
	}

	names := ResourceNames(result)
	written := make([]string, 0, len(names))
	for _, name := range names {
		path := filepath.Join(dir, name+".yaml")
		content := RenderPolicy(name, result)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return written, fmt.Errorf("writing %s: %w", path, err)
		}
		written = append(written, path)
	}
	return written, nil
}
