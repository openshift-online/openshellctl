package transfer

import (
	"io/fs"
	"os"
	"path/filepath"
)

// LstatFS is an fs.FS that can stat a path without following a final symlink.
type LstatFS interface {
	fs.FS
	Lstat(name string) (fs.FileInfo, error)
}

// ReadlinkFS is an fs.FS that can read a symlink's target.
type ReadlinkFS interface {
	fs.FS
	Readlink(name string) (string, error)
}

// lstatFS stats name without following a final symlink when fsys supports it,
// otherwise falls back to fs.Stat (which follows symlinks).
func lstatFS(fsys fs.FS, name string) (fs.FileInfo, error) {
	if l, ok := fsys.(LstatFS); ok {
		return l.Lstat(name)
	}
	return fs.Stat(fsys, name)
}

// readlinkFS reads name's symlink target when fsys supports it.
func readlinkFS(fsys fs.FS, name string) (string, error) {
	if r, ok := fsys.(ReadlinkFS); ok {
		return r.Readlink(name)
	}
	return "", &fs.PathError{Op: "readlink", Path: name, Err: fs.ErrInvalid}
}

// osDirFS is an fs.FS rooted at a directory that also supports Lstat/Readlink by
// resolving names against the real filesystem. It is used for uploads so that
// symlinks are preserved and directory metadata is faithful.
type osDirFS struct{ root string }

// OSFS returns an fs.FS rooted at root supporting Lstat and Readlink. root must
// be an absolute or working-directory-relative path. Names passed to the fs are
// slash-separated and relative to root (fs.FS convention).
func OSFS(root string) fs.FS { return osDirFS{root: root} }

func (o osDirFS) real(name string) string {
	return filepath.Join(o.root, filepath.FromSlash(name))
}

func (o osDirFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	return os.Open(o.real(name))
}

func (o osDirFS) Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}
	return os.Stat(o.real(name))
}

func (o osDirFS) Lstat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "lstat", Path: name, Err: fs.ErrInvalid}
	}
	return os.Lstat(o.real(name))
}

func (o osDirFS) Readlink(name string) (string, error) {
	if !fs.ValidPath(name) {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: fs.ErrInvalid}
	}
	return os.Readlink(o.real(name))
}

func (o osDirFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	return os.ReadDir(o.real(name))
}
