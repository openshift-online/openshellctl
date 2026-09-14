package transfer

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// gitFilteredEntries and its helpers (repo-root discovery, nested .gitignore
// compilation, the filtered walk) are real decision logic operating over an
// fs.FS, so they are tested directly against an OSFS-backed fixture tree — no
// SSH involved.

func TestGitFilteredEntries(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "repo", ".git", "HEAD"), "ref: refs/heads/main")
	writeFile(t, filepath.Join(root, "repo", ".gitignore"), "*.log\nbuild/\n")
	writeFile(t, filepath.Join(root, "repo", "main.go"), "package main")
	writeFile(t, filepath.Join(root, "repo", "debug.log"), "noise")
	writeFile(t, filepath.Join(root, "repo", "build", "out.bin"), "bin")
	writeFile(t, filepath.Join(root, "repo", "pkg", "util.go"), "package pkg")
	// A nested .gitignore that only applies within pkg/.
	writeFile(t, filepath.Join(root, "repo", "pkg", ".gitignore"), "*.tmp\n")
	writeFile(t, filepath.Join(root, "repo", "pkg", "scratch.tmp"), "x")
	writeFile(t, filepath.Join(root, "repo", "keep.tmp"), "top-level tmp is kept")

	c := &client{fsys: OSFS(root)}
	entries, ok, err := c.gitFilteredEntries("repo", "repo")
	if err != nil {
		t.Fatalf("gitFiltered: %v", err)
	}
	if !ok {
		t.Fatal("expected repo detected")
	}
	archives := make([]string, 0, len(entries))
	for _, e := range entries {
		archives = append(archives, e.archivePath)
	}
	sort.Strings(archives)
	joined := strings.Join(archives, ",")

	for _, excluded := range []string{"debug.log", "out.bin", ".git/", "scratch.tmp"} {
		if strings.Contains(joined, excluded) {
			t.Errorf("%q should be excluded: %v", excluded, archives)
		}
	}
	for _, included := range []string{"repo/main.go", "repo/pkg/util.go", "repo/keep.tmp"} {
		if !strings.Contains(joined, included) {
			t.Errorf("%q should be included: %v", included, archives)
		}
	}
}

func TestGitFilteredEntries_GitInfoExclude(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "r", ".git", "HEAD"), "ref")
	writeFile(t, filepath.Join(root, "r", ".git", "info", "exclude"), "secret.txt\n")
	writeFile(t, filepath.Join(root, "r", "secret.txt"), "shh")
	writeFile(t, filepath.Join(root, "r", "ok.txt"), "ok")

	c := &client{fsys: OSFS(root)}
	entries, ok, err := c.gitFilteredEntries("r", "r")
	if err != nil || !ok {
		t.Fatalf("gitFiltered: ok=%v err=%v", ok, err)
	}
	for _, e := range entries {
		if strings.Contains(e.archivePath, "secret.txt") {
			t.Errorf(".git/info/exclude entry should be excluded: %v", entries)
		}
	}
}

func TestGitFilteredEntries_NoRepo(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "plain", "a.txt"), "A")
	c := &client{fsys: OSFS(root)}
	_, ok, err := c.gitFilteredEntries("plain", "plain")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Error("expected no repo detected outside a git tree")
	}
}

func TestFindRepoRoot(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "r", ".git", "HEAD"), "ref")
	writeFile(t, filepath.Join(root, "r", "sub", "deep", "f.txt"), "x")

	c := &client{fsys: OSFS(root)}
	if got, ok := c.findRepoRoot("r/sub/deep"); !ok || got != "r" {
		t.Errorf("findRepoRoot(dir) = (%q,%v), want (r,true)", got, ok)
	}
	if got, ok := c.findRepoRoot("r/sub/deep/f.txt"); !ok || got != "r" {
		t.Errorf("findRepoRoot(file) = (%q,%v), want (r,true)", got, ok)
	}
	if _, ok := c.findRepoRoot("r"); !ok {
		t.Error("findRepoRoot at repo root should succeed")
	}
}

// TestUploadGitignoreFallback exercises the empty-filter fallback through the
// Upload shell (repo where everything is ignored → warning + unfiltered upload).
func TestUploadGitignoreFallback(t *testing.T) {
	src := t.TempDir()
	remote := t.TempDir()
	writeFile(t, filepath.Join(src, "r", ".git", "HEAD"), "ref")
	writeFile(t, filepath.Join(src, "r", ".gitignore"), "*\n")
	writeFile(t, filepath.Join(src, "r", "x.txt"), "X")

	c, _ := newTestClient(t, src, remote)
	var reports []string
	if err := c.Upload(context.Background(), "default", "sb", "r", "", true, func(s string) {
		reports = append(reports, s)
	}); err != nil {
		t.Fatalf("upload: %v", err)
	}
	warned := false
	for _, r := range reports {
		if strings.Contains(r, "excluded all files") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("expected fallback warning, got %v", reports)
	}
	if got, err := readFileAt(remote, "r/x.txt"); err != nil || string(got) != "X" {
		t.Errorf("fallback should upload unfiltered: got %q err %v", got, err)
	}
}
