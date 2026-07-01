package boothengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteConfigUsesAnonymousCookieWhenEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "BDConfig.json")
	if err := writeConfig(path, "", true); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got boothConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Cookie != anonymousCookie {
		t.Fatalf("Cookie = %q, want %q", got.Cookie, anonymousCookie)
	}
	if !got.AutoZip {
		t.Fatal("expected AutoZip to round-trip as true")
	}
}

func TestWriteConfigPreservesRealCookie(t *testing.T) {
	path := filepath.Join(t.TempDir(), "BDConfig.json")
	if err := writeConfig(path, "real-session-token", false); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got boothConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Cookie != "real-session-token" {
		t.Fatalf("Cookie = %q, want %q", got.Cookie, "real-session-token")
	}
	if got.AutoZip {
		t.Fatal("expected AutoZip to round-trip as false")
	}
}

func TestWriteConfigFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode bits aren't meaningful on Windows")
	}
	path := filepath.Join(t.TempDir(), "BDConfig.json")
	if err := writeConfig(path, "secret", false); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %v, want 0600", info.Mode().Perm())
	}
}
