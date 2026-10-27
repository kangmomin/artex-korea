package selfupdate

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestKoreaUpdatePathsIgnoreLegacyStagedFiles(t *testing.T) {
	for _, name := range []string{"artex", "artex.exe"} {
		p := koreaPaths(filepath.Join(t.TempDir(), name))
		ext := filepath.Ext(name)
		stem := strings.TrimSuffix(name, ext)
		if filepath.Base(p.New) != stem+".korea.new"+ext || filepath.Base(p.Old) != stem+".korea.old"+ext || filepath.Base(p.Marker) != stem+".korea.upgrade.json" || p.Sum != p.New+".sha256" {
			t.Fatalf("update files are not isolated from the original edition: %+v", p)
		}
		if filepath.Base(p.Current) != name {
			t.Fatal("changed current binary name")
		}
	}
}

func TestKoreanReleaseSource(t *testing.T) {
	old := Repo
	t.Cleanup(func() { Repo = old })
	for _, repo := range []string{"", "Autumn-27/artex", "jiwoochris/artex-global", "owner/artex-korea/extra", "owner?x/artex-korea"} {
		Repo = repo
		if _, err := latestReleaseURL(); err == nil {
			t.Fatalf("accepted unsafe/wrong edition source %q", repo)
		}
	}
	Repo = "example-owner/artex-korea"
	if got, err := latestReleaseURL(); err != nil || got != "https://api.github.com/repos/example-owner/artex-korea/releases/latest" {
		t.Fatalf("release source: %q %v", got, err)
	}
}
