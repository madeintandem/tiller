// Package selfupdate resolves tiller's GitHub Releases and swaps the running
// binary for a downloaded one. It is the engine of `tiller upgrade` (lite):
// resolve, download, verify, swap (docs/spec/distribution.md, Update).
package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// repo is the GitHub owner/name every request resolves against. Tiller has
// exactly one home; this never becomes a setting.
const repo = "madeintandem/tiller"

// defaultHTTPTimeout is the timeout every Client carries, whether built by
// NewClient or as a bare Client{} literal (a hung connection must never
// stall `tiller upgrade`, or a test built the same way, forever).
const defaultHTTPTimeout = 30 * time.Second

// defaultHTTPClient backs httpClient's fallback: a Client{} literal with no
// HTTP field still gets a timeout, rather than silently inheriting
// http.DefaultClient's no-timeout zero value.
var defaultHTTPClient = &http.Client{Timeout: defaultHTTPTimeout}

// Release is one GitHub release: its tag and the assets it carries.
type Release struct {
	Tag    string // e.g. "v0.9.5"
	Assets []Asset
}

// Asset is one release asset: its file name and its API url, the endpoint a
// token-authenticated download uses (the browser URL 404s on private repos).
type Asset struct {
	Name   string
	APIURL string
}

// Client talks to the GitHub API and download hosts. Zero-value bases mean
// the real GitHub; tests point them at httptest servers via the environment
// seams TILLER_UPGRADE_API_BASE and TILLER_UPGRADE_DOWNLOAD_BASE (internal
// test seams, not documented surface).
type Client struct {
	APIBase      string // default https://api.github.com
	DownloadBase string // default https://github.com
	Token        string // GITHUB_TOKEN when set
	HTTP         *http.Client
}

// NewClient builds a Client for real use: GITHUB_TOKEN and the two test
// seams come from the environment when set, and the HTTP client carries a
// 30 second timeout so a hung connection doesn't stall `tiller upgrade`
// forever.
func NewClient() *Client {
	return &Client{
		APIBase:      os.Getenv("TILLER_UPGRADE_API_BASE"),
		DownloadBase: os.Getenv("TILLER_UPGRADE_DOWNLOAD_BASE"),
		Token:        os.Getenv("GITHUB_TOKEN"),
		HTTP:         &http.Client{Timeout: defaultHTTPTimeout},
	}
}

// releasePayload is the slice of the GitHub release JSON this package reads:
// the tag and each asset's name and API url.
type releasePayload struct {
	Tag    string `json:"tag_name"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"assets"`
}

// Latest resolves the newest published release.
func (c *Client) Latest(ctx context.Context) (Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", c.apiBase(), repo)
	return c.getRelease(ctx, url)
}

// ReleaseByTag resolves one named release, e.g. "v0.9.5".
func (c *Client) ReleaseByTag(ctx context.Context, tag string) (Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/tags/%s", c.apiBase(), repo, tag)
	return c.getRelease(ctx, url)
}

// FetchAsset downloads one named asset from rel to dst. With a Token set, it
// looks the asset up in rel.Assets and GETs its API url with
// Accept: application/octet-stream, the one path that also reaches private
// repos; without a Token, it GETs the public browser download URL directly,
// which is enough for a public repo and 404s otherwise. Either way, a
// missing asset or a non-2xx response comes back as an error naming both the
// asset and the release tag.
func (c *Client) FetchAsset(ctx context.Context, rel Release, name, dst string) error {
	authed := c.Token != ""

	var url string
	if authed {
		asset, ok := findAsset(rel, name)
		if !ok {
			return fmt.Errorf("asset %s not found in release %s", name, rel.Tag)
		}
		url = asset.APIURL
	} else {
		url = fmt.Sprintf("%s/%s/releases/download/%s/%s", c.downloadBase(), repo, rel.Tag, name)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("building request for asset %s from release %s: %w", name, rel.Tag, err)
	}
	if authed {
		req.Header.Set("Accept", "application/octet-stream")
	}
	c.authorize(req)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("downloading asset %s from release %s: %w", name, rel.Tag, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("downloading asset %s from release %s: unexpected status %s", name, rel.Tag, resp.Status)
	}

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("creating %s for asset %s: %w", dst, name, err)
	}

	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		os.Remove(dst)
		return fmt.Errorf("writing asset %s from release %s to %s: %w", name, rel.Tag, dst, err)
	}

	if err := out.Close(); err != nil {
		os.Remove(dst)
		return fmt.Errorf("closing %s for asset %s: %w", dst, name, err)
	}
	return nil
}

// getRelease fetches and decodes one release document from url.
func (c *Client) getRelease(ctx context.Context, url string) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, fmt.Errorf("building request for %s: %w", url, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	c.authorize(req)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Release{}, fmt.Errorf("fetching %s: unexpected status %s", url, resp.Status)
	}

	var payload releasePayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Release{}, fmt.Errorf("decoding release from %s: %w", url, err)
	}

	rel := Release{Tag: payload.Tag}
	for _, a := range payload.Assets {
		rel.Assets = append(rel.Assets, Asset{Name: a.Name, APIURL: a.URL})
	}
	return rel, nil
}

// authorize sets the bearer header iff the client carries a token.
func (c *Client) authorize(req *http.Request) {
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
}

// apiBase returns APIBase, or the real GitHub API when it's unset.
func (c *Client) apiBase() string {
	if c.APIBase != "" {
		return c.APIBase
	}
	return "https://api.github.com"
}

// downloadBase returns DownloadBase, or the real GitHub site when it's unset.
func (c *Client) downloadBase() string {
	if c.DownloadBase != "" {
		return c.DownloadBase
	}
	return "https://github.com"
}

// httpClient returns HTTP, or the shared timeout-bearing default when it's
// unset.
func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return defaultHTTPClient
}

// findAsset looks name up among rel's assets.
func findAsset(rel Release, name string) (Asset, bool) {
	for _, a := range rel.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}
