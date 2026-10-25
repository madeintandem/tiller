package ui_test

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/serve"
	"github.com/madeintandem/tiller/ui"
)

func TestEmbeddedThirdPartyNoticesAreServed(t *testing.T) {
	assets, err := fs.Sub(ui.Dist, "dist")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(serve.Handler(t.TempDir(), assets, "test", serve.DefaultAuthority()))
	defer srv.Close()

	response, err := srv.Client().Get(srv.URL + "/THIRD_PARTY_NOTICES.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/plain") {
		t.Fatalf("notices returned %s (%s)", response.Status, response.Header.Get("Content-Type"))
	}
	for _, required := range []string{
		"Go standard library", "github.com/spf13/cobra", "github.com/spf13/pflag",
		"github.com/yuin/goldmark", "golang.org/x/sys", "golang.org/x/term", "gopkg.in/yaml.v3",
		"react ", "react-dom ", "scheduler ", "react-markdown ", "micromark ",
		"Copyright", "Permission is hereby granted", "SIL OPEN FONT LICENSE Version 1.1",
		"Neither the Font Software nor any of its individual components",
	} {
		if !strings.Contains(string(body), required) {
			t.Errorf("embedded notices missing %q", required)
		}
	}
}

func TestEmbeddedFontsHaveFullLicenses(t *testing.T) {
	for _, family := range []string{"Inter", "Space-Grotesk", "Space-Mono"} {
		t.Run(family, func(t *testing.T) {
			content, err := ui.Dist.ReadFile("dist/fonts/" + family + "-OFL.txt")
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range []string{"Copyright", "SIL OPEN FONT LICENSE Version 1.1", "PERMISSION", "TERMINATION", "DISCLAIMER"} {
				if !strings.Contains(string(content), required) {
					t.Errorf("font license missing %q", required)
				}
			}
		})
	}
}

// The ui build writes the same notices to the root (packed by GoReleaser),
// ui/public, and the embedded dist. A partial commit must not let them differ.
func TestCommittedNoticeCopiesMatchTheEmbeddedNotices(t *testing.T) {
	embedded, err := ui.Dist.ReadFile("dist/THIRD_PARTY_NOTICES.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../THIRD_PARTY_NOTICES.txt", "public/THIRD_PARTY_NOTICES.txt"} {
		copied, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(copied) != string(embedded) {
			t.Errorf("%s differs from ui/dist/THIRD_PARTY_NOTICES.txt; run `make ui` and commit every notice copy", path)
		}
	}
}
