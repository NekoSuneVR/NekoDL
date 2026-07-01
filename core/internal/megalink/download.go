package megalink

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
)

// FileMetadata is what Metadata/Download learn about a file before or
// while fetching it.
type FileMetadata struct {
	Name string
	Size int64
}

// Downloader resolves and downloads public mega.nz file links.
type Downloader struct {
	Client *http.Client

	// APIBaseURL overrides MEGA's API endpoint — for tests only.
	APIBaseURL string
}

func (d *Downloader) client() *http.Client {
	if d.Client != nil {
		return d.Client
	}
	return http.DefaultClient
}

func (d *Downloader) resolve(ctx context.Context, rawURL string) (fileInfo, FileKey, string, error) {
	handle, keyB64, err := ParseLink(rawURL)
	if err != nil {
		return fileInfo{}, FileKey{}, "", err
	}
	fk, err := deriveFileKey(keyB64)
	if err != nil {
		return fileInfo{}, FileKey{}, "", err
	}
	info, err := fetchFileInfo(ctx, d.client(), d.APIBaseURL, handle)
	if err != nil {
		return fileInfo{}, FileKey{}, "", err
	}
	return info, fk, handle, nil
}

func decodedName(info fileInfo, fk FileKey) (string, error) {
	attrBytes, err := base64.RawURLEncoding.DecodeString(info.Attributes)
	if err != nil {
		return "", fmt.Errorf("megalink: decoding attributes: %w", err)
	}
	return decryptAttributes(attrBytes, fk.AESKey)
}

// Metadata fetches a file's name and size without downloading it.
func (d *Downloader) Metadata(ctx context.Context, rawURL string) (FileMetadata, error) {
	info, fk, _, err := d.resolve(ctx, rawURL)
	if err != nil {
		return FileMetadata{}, err
	}
	name, err := decodedName(info, fk)
	if err != nil {
		return FileMetadata{}, err
	}
	return FileMetadata{Name: name, Size: info.Size}, nil
}

// Download fetches and decrypts a public mega.nz file to destPath.
//
// It does not verify MEGA's integrity MAC (FileKey.MetaMAC) — a
// bit-corrupted download isn't currently detected, only a wrong key (which
// fails attribute decryption with a clear error). It also downloads as a
// single connection; MEGA's temporary URLs weren't confirmed to support
// resumable Range requests, so this doesn't plug into the segmented/resumable
// httpengine the way plain HTTP and resolver-based downloads do. See TODO.md.
func (d *Downloader) Download(ctx context.Context, rawURL, destPath string) (FileMetadata, error) {
	info, fk, _, err := d.resolve(ctx, rawURL)
	if err != nil {
		return FileMetadata{}, err
	}
	name, err := decodedName(info, fk)
	if err != nil {
		return FileMetadata{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.DownloadURL, nil)
	if err != nil {
		return FileMetadata{}, err
	}
	resp, err := d.client().Do(req)
	if err != nil {
		return FileMetadata{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return FileMetadata{}, fmt.Errorf("megalink: unexpected status %d fetching ciphertext", resp.StatusCode)
	}

	plainReader, err := decryptingReader(resp.Body, fk)
	if err != nil {
		return FileMetadata{}, err
	}

	out, err := os.Create(destPath)
	if err != nil {
		return FileMetadata{}, err
	}
	defer out.Close()

	if _, err := io.Copy(out, plainReader); err != nil {
		return FileMetadata{}, fmt.Errorf("megalink: writing decrypted file: %w", err)
	}

	return FileMetadata{Name: name, Size: info.Size}, nil
}
