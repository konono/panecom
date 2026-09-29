package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const binaryName = "panecom"

type Updater struct {
	HTTPClient     HTTPClient
	CurrentVersion string
	GOOS           string
	GOARCH         string
	Stderr         io.Writer
	ExecPath       string
	BaseURL        string // override GitHub API base URL (for testing)
}

func Run(currentVersion string) error {
	u := &Updater{
		HTTPClient:     http.DefaultClient,
		CurrentVersion: currentVersion,
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		Stderr:         os.Stderr,
	}
	return u.Execute()
}

func (u *Updater) Execute() error {
	_, _ = fmt.Fprintln(u.Stderr, "Checking for updates...")

	release, err := u.fetchLatestRelease()
	if err != nil {
		return fmt.Errorf("checking latest release: %w", err)
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")

	newer, err := isNewer(latestVersion, u.CurrentVersion)
	if err != nil {
		return fmt.Errorf("comparing versions: %w", err)
	}
	if !newer {
		_, _ = fmt.Fprintf(u.Stderr, "panecom %s is already the latest version.\n", u.CurrentVersion)
		return nil
	}

	_, _ = fmt.Fprintf(u.Stderr, "Updating panecom: %s → %s\n", u.CurrentVersion, latestVersion)

	assetURL, err := FindAssetURL(release, u.GOOS, u.GOARCH)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(u.Stderr, "Downloading for %s/%s...\n", u.GOOS, u.GOARCH)
	archiveData, err := u.download(assetURL)
	if err != nil {
		return fmt.Errorf("downloading release: %w", err)
	}

	binaryData, err := extractBinaryFromTarGz(archiveData, binaryName)
	if err != nil {
		return fmt.Errorf("extracting binary: %w", err)
	}

	targetPath, err := u.executablePath()
	if err != nil {
		return fmt.Errorf("determining executable path: %w", err)
	}

	if err := replaceBinary(targetPath, binaryData); err != nil {
		return fmt.Errorf("replacing binary: %w", err)
	}

	_, _ = fmt.Fprintln(u.Stderr, "Updated successfully! Run 'panecom --version' to verify.")
	return nil
}

func (u *Updater) fetchLatestRelease() (*ReleaseInfo, error) {
	if u.BaseURL != "" {
		return fetchRelease(u.HTTPClient, u.BaseURL+"/repos/"+repoOwner+"/"+repoName+"/releases/latest")
	}
	return FetchLatestRelease(u.HTTPClient)
}

func (u *Updater) download(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := u.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func extractBinaryFromTarGz(archiveData []byte, targetBinary string) ([]byte, error) {
	gr, err := gzip.NewReader(bytes.NewReader(archiveData))
	if err != nil {
		return nil, fmt.Errorf("opening gzip: %w", err)
	}
	defer func() { _ = gr.Close() }()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar: %w", err)
		}

		if filepath.Base(hdr.Name) == targetBinary {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("reading binary from archive: %w", err)
			}
			return data, nil
		}
	}

	return nil, fmt.Errorf("binary %q not found in archive", targetBinary)
}

func (u *Updater) executablePath() (string, error) {
	if u.ExecPath != "" {
		return u.ExecPath, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

func replaceBinary(targetPath string, newBinary []byte) error {
	dir := filepath.Dir(targetPath)
	tmpFile, err := os.CreateTemp(dir, binaryName+".update.*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	defer func() {
		if tmpPath != "" {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(newBinary); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmpFile.Chmod(0755); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("setting permissions: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("renaming: %w", err)
	}

	tmpPath = ""
	return nil
}
