package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	repoOwner = "konono"
	repoName  = "panecom"
)

type ReleaseInfo struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func FetchLatestRelease(client HTTPClient) (*ReleaseInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoName)
	return fetchRelease(client, url)
}

func fetchRelease(client HTTPClient, url string) (*ReleaseInfo, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
	}

	var release ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("parsing release info: %w", err)
	}

	return &release, nil
}

func FindAssetURL(release *ReleaseInfo, goos, goarch string) (string, error) {
	expected := fmt.Sprintf("panecom_%s_%s.tar.gz", goos, goarch)
	for _, a := range release.Assets {
		if a.Name == expected {
			return a.BrowserDownloadURL, nil
		}
	}
	return "", fmt.Errorf("no release asset found for %s/%s (expected %s)", goos, goarch, expected)
}
