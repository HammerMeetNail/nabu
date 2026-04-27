package migrations

import (
	"embed"
	"io/fs"
)

//go:embed *.sql
var Assets embed.FS

func Names() ([]string, error) {
	entries, err := fs.ReadDir(Assets, ".")
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	return names, nil
}
