package selfupdate_test

// These tests pin the network half of internal/selfupdate
// (docs/spec/distribution.md):
// resolving a GitHub release by tag or as the latest,
// and fetching one named asset either through the token-authenticated API
// endpoint or the public browser download URL, mirroring install.sh's two
// fetch paths (lines 47-116) but with real encoding/json structs instead of
// string scraping. Every test drives Client against an httptest server, so a
// wrong path, header, or JSON field fails here before it ever reaches the
// real GitHub API.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/selfupdate"
	"github.com/stretchr/testify/require"
)

func TestLatestResolvesTheTagAndAssets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/repos/madeintandem/tiller/releases/latest", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tag_name": "v1.2.3",
			"assets": [
				{"name": "tiller_1.2.3_darwin_arm64.tar.gz", "url": "https://api.github.com/repos/madeintandem/tiller/releases/assets/111"},
				{"name": "checksums.txt", "url": "https://api.github.com/repos/madeintandem/tiller/releases/assets/222"}
			]
		}`))
	}))
	defer server.Close()

	client := &selfupdate.Client{APIBase: server.URL}
	rel, err := client.Latest(context.Background())
	require.NoError(t, err)
	require.Equal(t, "v1.2.3", rel.Tag)
	require.Equal(t, []selfupdate.Asset{
		{Name: "tiller_1.2.3_darwin_arm64.tar.gz", APIURL: "https://api.github.com/repos/madeintandem/tiller/releases/assets/111"},
		{Name: "checksums.txt", APIURL: "https://api.github.com/repos/madeintandem/tiller/releases/assets/222"},
	}, rel.Assets)
}

func TestReleaseByTagResolvesANamedRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/repos/madeintandem/tiller/releases/tags/v0.9.5", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tag_name": "v0.9.5",
			"assets": [
				{"name": "tiller_0.9.5_linux_amd64.tar.gz", "url": "https://api.github.com/repos/madeintandem/tiller/releases/assets/333"}
			]
		}`))
	}))
	defer server.Close()

	client := &selfupdate.Client{APIBase: server.URL}
	rel, err := client.ReleaseByTag(context.Background(), "v0.9.5")
	require.NoError(t, err)
	require.Equal(t, "v0.9.5", rel.Tag)
	require.Equal(t, []selfupdate.Asset{
		{Name: "tiller_0.9.5_linux_amd64.tar.gz", APIURL: "https://api.github.com/repos/madeintandem/tiller/releases/assets/333"},
	}, rel.Assets)
}

func TestLatestSendsTheBearerTokenWhenSet(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"tag_name": "v1.0.0", "assets": []}`))
	}))
	defer server.Close()

	client := &selfupdate.Client{APIBase: server.URL, Token: "sekrit"}
	_, err := client.Latest(context.Background())
	require.NoError(t, err)
	require.Equal(t, "Bearer sekrit", gotAuth)
}

func TestFetchAssetUsesTheAPIEndpointWithAToken(t *testing.T) {
	var gotAccept, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("archive-bytes"))
	}))
	defer server.Close()

	rel := selfupdate.Release{
		Tag: "v1.2.3",
		Assets: []selfupdate.Asset{
			{Name: "tiller_1.2.3_darwin_arm64.tar.gz", APIURL: server.URL + "/assets/111"},
		},
	}
	client := &selfupdate.Client{Token: "sekrit"}
	dst := filepath.Join(t.TempDir(), "out.tar.gz")

	err := client.FetchAsset(context.Background(), rel, "tiller_1.2.3_darwin_arm64.tar.gz", dst)
	require.NoError(t, err)
	require.Equal(t, "application/octet-stream", gotAccept)
	require.Equal(t, "Bearer sekrit", gotAuth)

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	require.Equal(t, "archive-bytes", string(got))
}

func TestFetchAssetUsesThePublicURLWithoutAToken(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte("archive-bytes"))
	}))
	defer server.Close()

	rel := selfupdate.Release{Tag: "v1.2.3"}
	client := &selfupdate.Client{DownloadBase: server.URL}
	dst := filepath.Join(t.TempDir(), "out.tar.gz")

	err := client.FetchAsset(context.Background(), rel, "tiller_1.2.3_darwin_arm64.tar.gz", dst)
	require.NoError(t, err)
	require.Equal(t, "/madeintandem/tiller/releases/download/v1.2.3/tiller_1.2.3_darwin_arm64.tar.gz", gotPath)

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	require.Equal(t, "archive-bytes", string(got))
}

func TestFetchAssetNamesTheMissingAsset(t *testing.T) {
	rel := selfupdate.Release{
		Tag: "v1.2.3",
		Assets: []selfupdate.Asset{
			{Name: "checksums.txt", APIURL: "https://example.com/repos/madeintandem/tiller/releases/assets/999"},
		},
	}
	client := &selfupdate.Client{Token: "sekrit"}
	dst := filepath.Join(t.TempDir(), "out.tar.gz")

	err := client.FetchAsset(context.Background(), rel, "tiller_1.2.3_darwin_arm64.tar.gz", dst)
	require.Error(t, err)
	require.ErrorContains(t, err, "tiller_1.2.3_darwin_arm64.tar.gz")
	require.ErrorContains(t, err, "v1.2.3")
}

func TestFetchAssetRemovesAPartialDownloadOnCopyFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Promise more bytes than are ever sent, then hijack the connection
		// and close it outright: resp.Body's read enforces the declared
		// Content-Length, so io.Copy on the client side fails partway
		// through, the same as a connection dropped mid-download.
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("only a few bytes"))
		hj, ok := w.(http.Hijacker)
		require.True(t, ok, "test server response writer must support hijacking")
		conn, _, err := hj.Hijack()
		require.NoError(t, err)
		_ = conn.Close()
	}))
	defer server.Close()

	rel := selfupdate.Release{Tag: "v1.2.3"}
	client := &selfupdate.Client{DownloadBase: server.URL}
	dst := filepath.Join(t.TempDir(), "out.tar.gz")

	err := client.FetchAsset(context.Background(), rel, "tiller_1.2.3_darwin_arm64.tar.gz", dst)
	require.Error(t, err)

	_, statErr := os.Stat(dst)
	require.True(t, os.IsNotExist(statErr), "expected %s to be removed after a failed download, stat err: %v", dst, statErr)
}

func TestNewClientReadsTheEnvironmentSeams(t *testing.T) {
	t.Setenv("TILLER_UPGRADE_API_BASE", "https://api.example.test")
	t.Setenv("TILLER_UPGRADE_DOWNLOAD_BASE", "https://dl.example.test")
	t.Setenv("GITHUB_TOKEN", "sekrit")

	client := selfupdate.NewClient()
	require.Equal(t, "https://api.example.test", client.APIBase)
	require.Equal(t, "https://dl.example.test", client.DownloadBase)
	require.Equal(t, "sekrit", client.Token)
	require.NotNil(t, client.HTTP)
	require.Equal(t, 30*time.Second, client.HTTP.Timeout)
}
