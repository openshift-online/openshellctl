package transfer

import (
	"archive/tar"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
)

// uploadEntry is one path to add to the archive: fsPath is the path within the
// source fs.FS, archivePath is the name it gets inside the tar.
type uploadEntry struct {
	fsPath      string
	archivePath string
}

// writeUploadArchive writes a tar of entries (already resolved, in order) to w
// over the source fs. Directories emit a "<name>/" entry; symlinks are preserved
// as symlink entries with the raw link target; regular files copy their bytes;
// any other type is an error (unsupported file type). Mirrors ssh.rs:648-751.
func writeUploadArchive(w io.Writer, fsys fs.FS, entries []uploadEntry) error {
	tw := tar.NewWriter(w)
	for _, e := range entries {
		if err := appendUploadPath(tw, fsys, e.fsPath, e.archivePath); err != nil {
			return err
		}
	}
	return tw.Close()
}

// appendUploadPath adds fsPath (dispatched on its symlink-metadata type) as
// archivePath, recursing into directories with a sorted walk.
func appendUploadPath(tw *tar.Writer, fsys fs.FS, fsPath, archivePath string) error {
	info, err := lstatFS(fsys, fsPath)
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", fsPath, err)
	}
	mode := info.Mode()
	switch {
	case mode&fs.ModeSymlink != 0:
		return appendUploadSymlink(tw, fsys, fsPath, archivePath, info)
	case mode.IsDir():
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("failed to add directory %s to tar archive: %w", archivePath, err)
		}
		hdr.Name = archivePath + "/"
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("failed to add directory %s to tar archive: %w", archivePath, err)
		}
		return appendUploadDirContents(tw, fsys, fsPath, archivePath)
	case mode.IsRegular():
		return appendUploadFile(tw, fsys, fsPath, archivePath, info)
	default:
		return fmt.Errorf("unsupported file type for upload: %s", fsPath)
	}
}

func appendUploadFile(tw *tar.Writer, fsys fs.FS, fsPath, archivePath string, info fs.FileInfo) error {
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("failed to add %s to tar archive: %w", archivePath, err)
	}
	hdr.Name = archivePath
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("failed to add %s to tar archive: %w", archivePath, err)
	}
	f, err := fsys.Open(fsPath)
	if err != nil {
		return fmt.Errorf("failed to add %s to tar archive: %w", archivePath, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("failed to add %s to tar archive: %w", archivePath, err)
	}
	return nil
}

// appendUploadSymlink preserves a symlink as a symlink entry with the raw link
// target (not dereferenced), size 0. Mirrors ssh.rs:727-751.
func appendUploadSymlink(tw *tar.Writer, fsys fs.FS, fsPath, archivePath string, info fs.FileInfo) error {
	target, err := readlinkFS(fsys, fsPath)
	if err != nil {
		return fmt.Errorf("failed to read symlink %s: %w", fsPath, err)
	}
	hdr, err := tar.FileInfoHeader(info, target)
	if err != nil {
		return fmt.Errorf("failed to add symlink %s to tar archive: %w", archivePath, err)
	}
	hdr.Name = archivePath
	hdr.Typeflag = tar.TypeSymlink
	hdr.Linkname = target
	hdr.Size = 0
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("failed to add symlink %s to tar archive: %w", archivePath, err)
	}
	return nil
}

// appendUploadDirContents reads fsPath, sorts children by name, and recurses.
func appendUploadDirContents(tw *tar.Writer, fsys fs.FS, fsPath, archivePath string) error {
	ents, err := fs.ReadDir(fsys, fsPath)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", fsPath, err)
	}
	sort.Slice(ents, func(i, j int) bool { return ents[i].Name() < ents[j].Name() })
	for _, ent := range ents {
		childFS := path.Join(fsPath, ent.Name())
		childArchive := path.Join(archivePath, ent.Name())
		if err := appendUploadPath(tw, fsys, childFS, childArchive); err != nil {
			return err
		}
	}
	return nil
}
