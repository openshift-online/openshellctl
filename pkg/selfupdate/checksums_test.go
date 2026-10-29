package selfupdate

import (
	"errors"
	"strings"
	"testing"
)

func TestAssetName(t *testing.T) {
	tests := []struct {
		goos    string
		goarch  string
		want    string
		wantErr bool
	}{
		{"linux", "amd64", "openshellctl_Linux_x86_64.tar.gz", false},
		{"linux", "arm64", "openshellctl_Linux_arm64.tar.gz", false},
		{"darwin", "amd64", "openshellctl_Darwin_x86_64.tar.gz", false},
		{"darwin", "arm64", "openshellctl_Darwin_arm64.tar.gz", false},
		{"windows", "amd64", "", true},
		{"linux", "386", "", true},
		{"freebsd", "amd64", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.goos+"/"+tt.goarch, func(t *testing.T) {
			got, err := AssetName(tt.goos, tt.goarch)
			if tt.wantErr {
				var e *ErrUnsupportedPlatform
				if !errors.As(err, &e) {
					t.Errorf("AssetName(%q, %q) error = %v, want ErrUnsupportedPlatform", tt.goos, tt.goarch, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("AssetName(%q, %q) = %q, want %q", tt.goos, tt.goarch, got, tt.want)
			}
		})
	}
}

func TestChecksumAssetName(t *testing.T) {
	tests := []struct {
		tag  string
		want string
	}{
		{"v0.1.5", "openshellctl_0.1.5_checksums.txt"},
		{"0.1.5", "openshellctl_0.1.5_checksums.txt"},
		{"v0.1.5-rc.1", "openshellctl_0.1.5-rc.1_checksums.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			got := ChecksumAssetName(tt.tag)
			if got != tt.want {
				t.Errorf("ChecksumAssetName(%q) = %q, want %q", tt.tag, got, tt.want)
			}
		})
	}
}

func TestParseChecksums(t *testing.T) {
	t.Run("two-space separator", func(t *testing.T) {
		input := "abc123  openshellctl_Linux_x86_64.tar.gz\ndef456  openshellctl_Darwin_arm64.tar.gz\n"
		got, err := ParseChecksums(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["openshellctl_Linux_x86_64.tar.gz"] != "abc123" {
			t.Errorf("Linux amd64 = %q, want abc123", got["openshellctl_Linux_x86_64.tar.gz"])
		}
		if got["openshellctl_Darwin_arm64.tar.gz"] != "def456" {
			t.Errorf("Darwin arm64 = %q, want def456", got["openshellctl_Darwin_arm64.tar.gz"])
		}
	})

	t.Run("binary-mode star separator", func(t *testing.T) {
		// shasum -b produces "<hex> *<filename>"
		input := "abc123 *openshellctl_Linux_x86_64.tar.gz\n"
		got, err := ParseChecksums(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["openshellctl_Linux_x86_64.tar.gz"] != "abc123" {
			t.Errorf("got %q, want abc123", got["openshellctl_Linux_x86_64.tar.gz"])
		}
	})

	t.Run("blank lines and CRLF tolerated", func(t *testing.T) {
		input := "\r\nabc123  foo.tar.gz\r\n\r\n"
		got, err := ParseChecksums(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["foo.tar.gz"] != "abc123" {
			t.Errorf("got %q, want abc123", got["foo.tar.gz"])
		}
	})

	t.Run("malformed hex rejected", func(t *testing.T) {
		input := "zzzzzz  foo.tar.gz\n"
		_, err := ParseChecksums(strings.NewReader(input))
		if err == nil {
			t.Error("expected error for malformed hex, got nil")
		}
	})

	t.Run("empty reader returns empty map", func(t *testing.T) {
		got, err := ParseChecksums(strings.NewReader(""))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %v, want empty map", got)
		}
	})

	t.Run("missing filename rejected", func(t *testing.T) {
		input := "abc123\n"
		_, err := ParseChecksums(strings.NewReader(input))
		if err == nil {
			t.Error("expected error for missing filename, got nil")
		}
	})
}

func TestFindAsset(t *testing.T) {
	rel := Release{Assets: []Asset{
		{Name: "openshellctl_Linux_x86_64.tar.gz", BrowserDownloadURL: "https://github.com/example/v1"},
		{Name: "openshellctl_0.1.5_checksums.txt", BrowserDownloadURL: "https://github.com/example/cs"},
	}}

	t.Run("found", func(t *testing.T) {
		a, ok := FindAsset(rel, "openshellctl_Linux_x86_64.tar.gz")
		if !ok {
			t.Fatal("FindAsset returned not found")
		}
		if a.BrowserDownloadURL != "https://github.com/example/v1" {
			t.Errorf("URL = %q", a.BrowserDownloadURL)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, ok := FindAsset(rel, "nonexistent.tar.gz")
		if ok {
			t.Error("FindAsset returned ok for nonexistent asset")
		}
	})
}
