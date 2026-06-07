package app

import (
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/heurema/pactum/internal/projectmap"
)

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// writeJSONResponse encodes value as indented JSON to stdout. It is the shared
// formatter for every command's --json output.
func writeJSONResponse(stdout io.Writer, value any) error {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func readMapManifest(path string) (projectmap.Manifest, error) {
	var manifest projectmap.Manifest
	if err := readJSON(path, &manifest); err != nil {
		return projectmap.Manifest{}, err
	}
	if manifest.RunID == "" {
		return projectmap.Manifest{}, errors.New("project map manifest is incomplete")
	}
	return manifest, nil
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
