package megalink

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NekoSuneVR/NekoDL/core/internal/task"
)

// mockMegaServers builds a fake MEGA API + ciphertext server pair for a
// known plaintext, mirroring TestFullDownloadPipeline's approach, and
// returns the link URL to use against them.
func mockMegaServers(t *testing.T, plaintext []byte, filename string) (linkURL string, apiBaseURL string, cleanup func()) {
	t.Helper()

	words := []uint32{
		0x01020304, 0x05060708, 0x090A0B0C, 0x0D0E0F10,
		0x11121314, 0x15161718, 0x191A1B1C, 0x1D1E1F20,
	}
	rawKey := a32ToBytes(words)
	keyB64 := base64.RawURLEncoding.EncodeToString(rawKey)

	fk, err := deriveFileKey(keyB64)
	if err != nil {
		t.Fatalf("deriveFileKey: %v", err)
	}

	block, err := aes.NewCipher(fk.AESKey)
	if err != nil {
		t.Fatalf("aes.NewCipher: %v", err)
	}
	ciphertext := make([]byte, len(plaintext))
	cipher.NewCTR(block, fk.IV).XORKeyStream(ciphertext, plaintext)

	attrPayload := []byte(fmt.Sprintf(`MEGA{"n":"%s"}`, filename))
	for len(attrPayload)%aes.BlockSize != 0 {
		attrPayload = append(attrPayload, 0)
	}
	encryptedAttrs := make([]byte, len(attrPayload))
	cipher.NewCBCEncrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(encryptedAttrs, attrPayload)
	attrsB64 := base64.RawURLEncoding.EncodeToString(encryptedAttrs)

	ciphertextSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(ciphertext)
	}))
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := []map[string]any{{"g": ciphertextSrv.URL, "s": len(plaintext), "at": attrsB64}}
		data, _ := json.Marshal(resp)
		_, _ = w.Write(data)
	}))

	linkURL = fmt.Sprintf("https://mega.nz/file/testhandle#%s", keyB64)
	return linkURL, apiSrv.URL, func() {
		ciphertextSrv.Close()
		apiSrv.Close()
	}
}

func waitForStatus(t *testing.T, tk *Task, want task.Status, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if tk.Status() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for status %s, got %s", want, tk.Status())
}

func TestTaskDownloadsAndCompletes(t *testing.T) {
	plaintext := bytes.Repeat([]byte("mega task lifecycle test data. "), 300)
	linkURL, apiBase, cleanup := mockMegaServers(t, plaintext, "lifecycle-test.bin")
	defer cleanup()

	destDir := t.TempDir()
	tk := NewTask("task1", linkURL, destDir, &Downloader{APIBaseURL: apiBase})

	if err := tk.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitForStatus(t, tk, task.StatusComplete, 5*time.Second)

	progress := tk.Progress()
	if progress.DownloadedBytes != int64(len(plaintext)) {
		t.Fatalf("DownloadedBytes = %d, want %d", progress.DownloadedBytes, len(plaintext))
	}
	if progress.TotalBytes != int64(len(plaintext)) {
		t.Fatalf("TotalBytes = %d, want %d", progress.TotalBytes, len(plaintext))
	}

	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 downloaded file, got %d", len(entries))
	}
	if want := "task1-lifecycle-test.bin"; entries[0].Name() != want {
		t.Fatalf("got filename %q, want %q", entries[0].Name(), want)
	}

	got, err := os.ReadFile(filepath.Join(destDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatal("downloaded content does not match the original plaintext")
	}
}

func TestTaskFailsOnBadLink(t *testing.T) {
	tk := NewTask("task2", "https://mega.nz/file/x#y", t.TempDir(), &Downloader{APIBaseURL: "http://127.0.0.1:1"})

	if err := tk.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitForStatus(t, tk, task.StatusError, 5*time.Second)

	if tk.Err() == nil {
		t.Fatal("expected Err() to report a reason")
	}
}

func TestTaskCancel(t *testing.T) {
	plaintext := bytes.Repeat([]byte("x"), 5<<20) // large enough that cancel has time to land mid-transfer
	linkURL, apiBase, cleanup := mockMegaServers(t, plaintext, "cancel-test.bin")
	defer cleanup()

	tk := NewTask("task3", linkURL, t.TempDir(), &Downloader{APIBaseURL: apiBase})
	if err := tk.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if err := tk.Cancel(); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if got := tk.Status(); got != task.StatusCancelled {
		t.Fatalf("expected StatusCancelled, got %s", got)
	}
}
