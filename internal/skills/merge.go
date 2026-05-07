package skills

import (
	"io"
	"os"
	"path/filepath"
)

// Merge clears destDir and copies each discovered skill's directory tree into
// it. Skills are processed in reverse-slice order so the first occurrence in
// the input (highest precedence) is copied last and wins on name collision —
// matching Discover's "first occurrence wins" precedence model.
//
// destDir is fully removed and recreated on each call so stale skills from
// previous runs (e.g. a built-in removed in a binary upgrade) do not linger.
func Merge(destDir string, discovered []Skill) error {
	if err := os.RemoveAll(destDir); err != nil {
		return err
	}
	if err := os.MkdirAll(destDir, 0700); err != nil {
		return err
	}
	for i := len(discovered) - 1; i >= 0; i-- {
		s := discovered[i]
		srcDir := filepath.Dir(s.Path)
		dstDir := filepath.Join(destDir, s.Name)
		if err := os.MkdirAll(dstDir, 0700); err != nil {
			return err
		}
		if err := copyDir(srcDir, dstDir); err != nil {
			return err
		}
	}
	return nil
}

func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := os.MkdirAll(dstPath, 0700); err != nil {
				return err
			}
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
