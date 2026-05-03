// Package builtin embeds Pitú's built-in runtime skills and unpacks them into
// the runtime skills merge directory at startup.
package builtin

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed assets
var assets embed.FS

// Unpack copies every embedded built-in runtime skill into destDir.
// destDir is created if it does not exist. Existing files in destDir are
// overwritten — callers that need a clean state should remove destDir first
// (the harness clears ~/.pitu/data/skills/ on startup before unpacking).
func Unpack(destDir string) error {
	return fs.WalkDir(assets, "assets", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(path, "assets")
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			return os.MkdirAll(destDir, 0700)
		}
		target := filepath.Join(destDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		data, err := assets.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0600)
	})
}
