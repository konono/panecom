package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func createTestTarGz(t *testing.T, filename string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	hdr := &tar.Header{
		Name: filename,
		Mode: 0755,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractBinaryFromTarGz(t *testing.T) {
	content := []byte("#!/bin/sh\necho hello\n")
	archive := createTestTarGz(t, "panecom", content)

	data, err := extractBinaryFromTarGz(archive, "panecom")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, content) {
		t.Errorf("extracted content mismatch")
	}
}

func TestExtractBinaryFromTarGzNotFound(t *testing.T) {
	archive := createTestTarGz(t, "other-binary", []byte("data"))

	_, err := extractBinaryFromTarGz(archive, "panecom")
	if err == nil {
		t.Error("expected error for missing binary")
	}
}

func TestReplaceBinary(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "panecom")

	if err := os.WriteFile(targetPath, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}

	newContent := []byte("new-binary-content")
	if err := replaceBinary(targetPath, newContent); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, newContent) {
		t.Error("binary content was not replaced")
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0111 == 0 {
		t.Error("binary should be executable")
	}
}

func TestFindAssetURL(t *testing.T) {
	release := &ReleaseInfo{
		TagName: "v0.3.0",
		Assets: []Asset{
			{Name: "panecom_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.com/linux_amd64.tar.gz"},
			{Name: "panecom_darwin_arm64.tar.gz", BrowserDownloadURL: "https://example.com/darwin_arm64.tar.gz"},
		},
	}

	url, err := FindAssetURL(release, "darwin", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://example.com/darwin_arm64.tar.gz" {
		t.Errorf("unexpected URL: %s", url)
	}

	_, err = FindAssetURL(release, "windows", "amd64")
	if err == nil {
		t.Error("expected error for missing platform")
	}
}

func TestExecuteNewerVersionUpdates(t *testing.T) {
	newBinary := []byte("#!/bin/sh\necho new-version\n")
	archive := createTestTarGz(t, "panecom", newBinary)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/konono/panecom/releases/latest":
			release := map[string]interface{}{
				"tag_name": "v0.3.0",
				"assets": []map[string]string{
					{
						"name":                 "panecom_testOS_testArch.tar.gz",
						"browser_download_url": fmt.Sprintf("http://%s/download/asset.tar.gz", r.Host),
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(release)
		case "/download/asset.tar.gz":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	execPath := filepath.Join(dir, "panecom")
	if err := os.WriteFile(execPath, []byte("old-binary"), 0755); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	u := &Updater{
		HTTPClient:     srv.Client(),
		CurrentVersion: "0.2.0",
		GOOS:           "testOS",
		GOARCH:         "testArch",
		Stderr:         &stderr,
		ExecPath:       execPath,
		BaseURL:        srv.URL,
	}

	if err := u.Execute(); err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}

	got, err := os.ReadFile(execPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newBinary) {
		t.Errorf("binary not updated: got %q, want %q", string(got), string(newBinary))
	}

	output := stderr.String()
	if !bytes.Contains([]byte(output), []byte("0.2.0 → 0.3.0")) {
		t.Errorf("expected version transition in output, got: %s", output)
	}
	if !bytes.Contains([]byte(output), []byte("Updated successfully")) {
		t.Errorf("expected success message in output, got: %s", output)
	}
}

func TestExecuteAlreadyLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/konono/panecom/releases/latest":
			release := map[string]interface{}{
				"tag_name": "v0.2.0",
				"assets":   []map[string]string{},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(release)
		default:
			t.Errorf("unexpected request to %s — should not fetch asset when already latest", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	execPath := filepath.Join(dir, "panecom")
	originalContent := []byte("original-binary")
	if err := os.WriteFile(execPath, originalContent, 0755); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	u := &Updater{
		HTTPClient:     srv.Client(),
		CurrentVersion: "0.2.0",
		GOOS:           "testOS",
		GOARCH:         "testArch",
		Stderr:         &stderr,
		ExecPath:       execPath,
		BaseURL:        srv.URL,
	}

	if err := u.Execute(); err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}

	got, err := os.ReadFile(execPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, originalContent) {
		t.Error("binary should not have been modified when already latest")
	}

	output := stderr.String()
	if !bytes.Contains([]byte(output), []byte("already the latest")) {
		t.Errorf("expected 'already the latest' message, got: %s", output)
	}
}
