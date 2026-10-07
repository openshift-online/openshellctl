package gatewayconfig

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestExists_NotFound(t *testing.T) {
	env := Env{UserFS: fstest.MapFS{}}
	ok, err := Exists(env, "nope")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected ok=false for an unregistered gateway")
	}
}

func TestExists_Found(t *testing.T) {
	env := Env{UserFS: mapFSWith(map[string]string{
		"gateways/rosa/metadata.json": md("rosa", "https://gw"),
	})}
	ok, err := Exists(env, "rosa")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("expected ok=true for a registered gateway")
	}
}

func TestExists_InvalidNamePropagatesError(t *testing.T) {
	env := Env{UserFS: fstest.MapFS{}}
	_, err := Exists(env, "has/slash")
	var invalid *InvalidGatewayNameError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want InvalidGatewayNameError", err)
	}
}

// permCapturingWriter wraps memWriter (lastsandbox_test.go) to also record
// the fs.FileMode each WriteFile call was given.
type permCapturingWriter struct {
	*memWriter
	perms map[string]fs.FileMode
}

func newPermCapturingWriter() *permCapturingWriter {
	return &permCapturingWriter{memWriter: newMemWriter(), perms: map[string]fs.FileMode{}}
}

func (w *permCapturingWriter) WriteFile(rel string, data []byte, perm fs.FileMode) error {
	w.perms[rel] = perm
	return w.memWriter.WriteFile(rel, data, perm)
}

func TestWriteGateway_WritesMetadataAt0600(t *testing.T) {
	w := newPermCapturingWriter()
	env := Env{UserFS: fstest.MapFS{}}
	m := Metadata{Name: "rosa", GatewayEndpoint: "https://gw", IsRemote: true}

	if err := WriteGateway(w, env, "rosa", m); err != nil {
		t.Fatalf("WriteGateway: %v", err)
	}
	rel := "gateways/rosa/metadata.json"
	data, ok := w.files[rel]
	if !ok {
		t.Fatalf("metadata.json not written at %q", rel)
	}
	if w.perms[rel] != 0o600 {
		t.Errorf("perm = %o, want 0600", w.perms[rel])
	}
	got, err := ParseMetadata("rosa", data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "rosa" || got.GatewayEndpoint != "https://gw" {
		t.Errorf("written metadata = %+v", got)
	}
}

func TestWriteGateway_AlreadyExists(t *testing.T) {
	w := newPermCapturingWriter()
	env := Env{UserFS: mapFSWith(map[string]string{
		"gateways/rosa/metadata.json": md("rosa", "https://gw"),
	})}
	err := WriteGateway(w, env, "rosa", Metadata{Name: "rosa", GatewayEndpoint: "https://other"})
	var exists *GatewayExistsError
	if !errors.As(err, &exists) {
		t.Fatalf("err = %v, want GatewayExistsError", err)
	}
	if !errors.Is(err, ErrGatewayExists) {
		t.Error("err should match ErrGatewayExists")
	}
	if len(w.files) != 0 {
		t.Error("WriteGateway must not write when the gateway already exists")
	}
}

func TestWriteGateway_InvalidName(t *testing.T) {
	w := newPermCapturingWriter()
	env := Env{UserFS: fstest.MapFS{}}
	err := WriteGateway(w, env, "has/slash", Metadata{Name: "has/slash"})
	var invalid *InvalidGatewayNameError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want InvalidGatewayNameError", err)
	}
}

// TestWriteGateway_RealOSWriter_PermsAndAtomicity exercises WriteGateway
// against the real OSWriter (osenv.go) on a real temp filesystem, to confirm
// end to end: 0600 file, 0700 parent dir, and that the write is atomic (no
// partial file ever observable — OSWriter.WriteFile already does temp+rename;
// this pins WriteGateway actually drives that real primitive correctly).
func TestWriteGateway_RealOSWriter_PermsAndAtomicity(t *testing.T) {
	root := t.TempDir()
	w := &OSWriter{Root: root}
	env := Env{UserFS: os.DirFS(root)}

	m := Metadata{Name: "rosa", GatewayEndpoint: "https://gw", IsRemote: true}
	if err := WriteGateway(w, env, "rosa", m); err != nil {
		t.Fatalf("WriteGateway: %v", err)
	}

	path := filepath.Join(root, "gateways", "rosa", "metadata.json")
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("metadata.json not written: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("file perm = %o, want 0600", fi.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Join(root, "gateways", "rosa"))
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Errorf("dir perm = %o, want 0700", dirInfo.Mode().Perm())
	}

	// No leftover temp files from the atomic write.
	entries, err := os.ReadDir(filepath.Join(root, "gateways", "rosa"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "metadata.json" {
			t.Errorf("unexpected leftover file: %s", e.Name())
		}
	}
}
