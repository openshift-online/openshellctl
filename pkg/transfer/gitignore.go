package transfer

import (
	"io/fs"
	"path"
	"sort"
	"strings"

	gitignore "github.com/sabhiram/go-gitignore"
)

// gitFilteredEntries returns the upload entries for a directory with gitignore
// filtering applied (gitignore-compatible, not git-index-identical; §2 decision
// 8). ok is false when local is not inside a git repo (caller falls back to an
// unfiltered upload). Filtering honours nested .gitignore files, .git/info/exclude,
// and always excludes .git/. Mirrors the intent of run.rs:5616-5702 without
// shelling out to git.
func (c *client) gitFilteredEntries(local, archivePrefix string) ([]uploadEntry, bool, error) {
	root, ok := c.findRepoRoot(local)
	if !ok {
		return nil, false, nil
	}

	matcher, err := c.buildIgnoreMatcher(root)
	if err != nil {
		return nil, false, err
	}

	var files []string
	err = walkFilesSorted(c.fsys, local, func(rel string) error {
		// rel is relative to the fs root; compute the repo-relative path for the
		// matcher and the local-relative path for the archive.
		repoRel := trimPrefixSlash(rel, root)
		if repoRel == "" {
			return nil
		}
		if isUnderGitDir(repoRel) {
			return nil
		}
		if matcher != nil && matcher.MatchesPath(repoRel) {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, false, err
	}

	entries := make([]uploadEntry, 0, len(files))
	for _, f := range files {
		archive := trimPrefixSlash(f, local)
		if archivePrefix != "." && archivePrefix != "" {
			archive = path.Join(archivePrefix, archive)
		}
		entries = append(entries, uploadEntry{fsPath: f, archivePath: archive})
	}
	return entries, true, nil
}

// findRepoRoot walks up from local (within the fs) looking for a ".git" entry,
// returning the repo root path (fs-relative) or ok=false.
func (c *client) findRepoRoot(local string) (string, bool) {
	dir := local
	if info, err := lstatFS(c.fsys, local); err == nil && !info.IsDir() {
		dir = path.Dir(local)
	}
	for {
		gitPath := path.Join(dir, ".git")
		if gitPath == ".git" || gitPath == "/.git" {
			if _, err := lstatFS(c.fsys, ".git"); err == nil {
				return dirOrDot(dir), true
			}
		}
		if _, err := lstatFS(c.fsys, gitPath); err == nil {
			return dir, true
		}
		parent := path.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// buildIgnoreMatcher compiles the repo's ignore rules: all nested .gitignore
// files plus .git/info/exclude. Patterns are made repo-relative so nested
// .gitignore directories anchor correctly.
func (c *client) buildIgnoreMatcher(root string) (*gitignore.GitIgnore, error) {
	var lines []string

	addFile := func(fsPath, dirRel string) {
		data, err := fs.ReadFile(c.fsys, fsPath)
		if err != nil {
			return
		}
		for _, ln := range strings.Split(string(data), "\n") {
			p := gitignorePatternToRepoRel(ln, dirRel)
			if p != "" {
				lines = append(lines, p)
			}
		}
	}

	// .git/info/exclude at the repo root.
	addFile(joinRel(root, ".git/info/exclude"), "")

	// Nested .gitignore files.
	_ = walkFilesSorted(c.fsys, dirOrDot(root), func(rel string) error {
		if path.Base(rel) == ".gitignore" {
			dirRel := trimPrefixSlash(path.Dir(rel), root)
			addFile(rel, dirRel)
		}
		return nil
	})

	if len(lines) == 0 {
		return nil, nil
	}
	return gitignore.CompileIgnoreLines(lines...), nil
}

// gitignorePatternToRepoRel rewrites a .gitignore line found in dirRel to be
// repo-relative. Comments and blanks return "". Negations and anchored patterns
// are preserved.
func gitignorePatternToRepoRel(line, dirRel string) string {
	trimmed := strings.TrimRight(line, "\r")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return ""
	}
	if dirRel == "" {
		return trimmed
	}
	neg := ""
	body := trimmed
	if strings.HasPrefix(body, "!") {
		neg = "!"
		body = body[1:]
	}
	// An anchored pattern (leading "/") is relative to dirRel; a non-anchored
	// pattern matches at any depth below dirRel, so we scope it to dirRel/**.
	if strings.HasPrefix(body, "/") {
		return neg + path.Join(dirRel, body)
	}
	return neg + path.Join(dirRel, "**", body)
}

func isUnderGitDir(repoRel string) bool {
	return repoRel == ".git" || strings.HasPrefix(repoRel, ".git/")
}

// walkFilesSorted walks dir within fsys, calling fn with each regular-file and
// symlink path (fs-relative), in sorted order. Directories are descended into
// (sorted) but not passed to fn; .git directories are skipped.
func walkFilesSorted(fsys fs.FS, dir string, fn func(rel string) error) error {
	ents, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return err
	}
	sort.Slice(ents, func(i, j int) bool { return ents[i].Name() < ents[j].Name() })
	for _, ent := range ents {
		child := path.Join(dir, ent.Name())
		info, err := lstatFS(fsys, child)
		if err != nil {
			continue
		}
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			if err := fn(child); err != nil {
				return err
			}
		case info.IsDir():
			if ent.Name() == ".git" {
				continue
			}
			if err := walkFilesSorted(fsys, child, fn); err != nil {
				return err
			}
		case info.Mode().IsRegular():
			if err := fn(child); err != nil {
				return err
			}
		}
	}
	return nil
}

// trimPrefixSlash returns child relative to base ("" when child == base).
func trimPrefixSlash(child, base string) string {
	base = dirOrDot(base)
	if base == "." {
		return child
	}
	if child == base {
		return ""
	}
	return strings.TrimPrefix(child, base+"/")
}

func joinRel(base, rel string) string {
	if base == "." || base == "" {
		return rel
	}
	return path.Join(base, rel)
}

func dirOrDot(d string) string {
	if d == "" {
		return "."
	}
	return d
}
