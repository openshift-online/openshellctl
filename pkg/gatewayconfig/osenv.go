package gatewayconfig

import (
	"io/fs"
	"os"
	"path/filepath"
)

// NewOSEnv builds an Env backed by the real filesystem: Getenv = os.Getenv, and
// the user/system FS rooted at the resolved config trees. A tree whose root does
// not exist yields a nil FS (reads simply miss), which the loaders tolerate.
func NewOSEnv() (Env, error) {
	getenv := os.Getenv
	userDir, err := UserConfigDir(getenv)
	if err != nil {
		return Env{}, err
	}
	sysDir := SystemBaseDir(getenv)

	return Env{
		Getenv:  getenv,
		UserFS:  dirFSIfExists(userDir),
		SysFS:   dirFSIfExists(sysDir),
		UserDir: userDir,
		SysDir:  sysDir,
	}, nil
}

// UserConfigDirPath returns the absolute user config dir (for the OSWriter root).
func UserConfigDirPath(getenv func(string) string) (string, error) {
	return UserConfigDir(getenv)
}

func dirFSIfExists(dir string) fs.FS {
	if dir == "" {
		return nil
	}
	if _, err := os.Stat(dir); err != nil {
		return nil
	}
	return os.DirFS(dir)
}

// OSWriter writes to the real user config tree, rooted at Root. Files are
// created 0600 and parent directories 0700, via atomic temp+rename.
type OSWriter struct{ Root string }

// NewOSWriter builds an OSWriter rooted at the user config dir.
func NewOSWriter() (*OSWriter, error) {
	root, err := UserConfigDir(os.Getenv)
	if err != nil {
		return nil, err
	}
	return &OSWriter{Root: root}, nil
}

// WriteFile atomically writes data to Root/relPath with the given perm, creating
// parent dirs 0700.
func (w *OSWriter) WriteFile(relPath string, data []byte, perm fs.FileMode) error {
	full := filepath.Join(w.Root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(full), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op after a successful rename

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, full)
}

// Remove deletes Root/relPath; a missing file is not an error.
func (w *OSWriter) Remove(relPath string) error {
	full := filepath.Join(w.Root, filepath.FromSlash(relPath))
	err := os.Remove(full)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ReadFile reads Root/relPath.
func (w *OSWriter) ReadFile(relPath string) ([]byte, error) {
	full := filepath.Join(w.Root, filepath.FromSlash(relPath))
	return os.ReadFile(full)
}
