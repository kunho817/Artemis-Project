package memory

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// EnsureCTags resolves a ctags binary path using 4-tier fallback:
//  1. Custom path from config (if set)
//  2. System PATH lookup ("universal-ctags" then "ctags")
//  3. Cached download in dataDir/ctags[.exe]
//  4. Auto-download from GitHub Releases -> cache
//
// Returns binary path and error. Error only if all tiers fail.
func EnsureCTags(customPath, dataDir string) (string, error) {
	var lastErr error

	// Tier 1: custom path
	if customPath != "" {
		if fi, err := os.Stat(customPath); err == nil && !fi.IsDir() {
			return customPath, nil
		}
		lastErr = fmt.Errorf("configured ctags path is invalid: %s", customPath)
	}

	// Tier 2: system PATH lookup
	if path, err := exec.LookPath("universal-ctags"); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("ctags"); err == nil {
		return path, nil
	}

	// Tier 3: cached binary
	cachePath := filepath.Join(dataDir, ctagsBinaryName())
	if fi, err := os.Stat(cachePath); err == nil && !fi.IsDir() {
		return cachePath, nil
	}

	// Tier 4: auto-download into cache
	if err := downloadCTags(cachePath); err == nil {
		return cachePath, nil
	} else {
		lastErr = err
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("ctags binary not found")
	}
	return "", fmt.Errorf("failed to resolve ctags binary: %w", lastErr)
}

func ctagsBinaryName() string {
	if runtime.GOOS == "windows" {
		return "ctags.exe"
	}
	return "ctags"
}

func downloadCTags(destPath string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("auto-download is only supported on Windows in Phase 3 MVP; install universal-ctags manually (Linux: apt install universal-ctags, macOS: brew install universal-ctags)")
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create ctags cache directory: %w", err)
	}

	const windowsURL = "https://github.com/universal-ctags/ctags-win32/releases/latest/download/ctags-x64.zip"
	resp, err := http.Get(windowsURL)
	if err != nil {
		return fmt.Errorf("download ctags archive: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download ctags archive: unexpected status %s", resp.Status)
	}

	tmpZip, err := os.CreateTemp(filepath.Dir(destPath), "ctags-*.zip")
	if err != nil {
		return fmt.Errorf("create temp archive file: %w", err)
	}
	tmpZipPath := tmpZip.Name()
	defer os.Remove(tmpZipPath)

	if _, err := io.Copy(tmpZip, resp.Body); err != nil {
		tmpZip.Close()
		return fmt.Errorf("write temp archive file: %w", err)
	}
	if err := tmpZip.Close(); err != nil {
		return fmt.Errorf("close temp archive file: %w", err)
	}

	zr, err := zip.OpenReader(tmpZipPath)
	if err != nil {
		return fmt.Errorf("open zip archive: %w", err)
	}
	defer zr.Close()

	binName := ctagsBinaryName()
	for _, f := range zr.File {
		if filepath.Base(f.Name) != binName {
			continue
		}

		src, err := f.Open()
		if err != nil {
			return fmt.Errorf("open binary inside archive: %w", err)
		}

		dst, err := os.Create(destPath)
		if err != nil {
			src.Close()
			return fmt.Errorf("create cached ctags binary: %w", err)
		}

		_, copyErr := io.Copy(dst, src)
		closeSrcErr := src.Close()
		closeDstErr := dst.Close()
		if copyErr != nil {
			return fmt.Errorf("extract ctags binary: %w", copyErr)
		}
		if closeSrcErr != nil {
			return fmt.Errorf("close archive binary stream: %w", closeSrcErr)
		}
		if closeDstErr != nil {
			return fmt.Errorf("close cached ctags binary: %w", closeDstErr)
		}

		if runtime.GOOS != "windows" {
			if err := os.Chmod(destPath, 0o755); err != nil {
				return fmt.Errorf("set executable permission: %w", err)
			}
		}

		return nil
	}

	return fmt.Errorf("ctags executable not found in downloaded archive")
}
