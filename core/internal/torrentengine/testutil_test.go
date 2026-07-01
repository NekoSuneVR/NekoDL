package torrentengine

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

// buildTestTorrent creates single-file torrent metadata for filePath,
// hashing its actual bytes — this is a real, valid torrent, not a stub.
func buildTestTorrent(t *testing.T, filePath string) metainfo.MetaInfo {
	t.Helper()

	info := metainfo.Info{PieceLength: 64 * 1024}
	if err := info.BuildFromFilePath(filePath); err != nil {
		t.Fatalf("BuildFromFilePath: %v", err)
	}
	if err := info.GeneratePieces(func(metainfo.FileInfo) (io.ReadCloser, error) {
		return os.Open(filePath)
	}); err != nil {
		t.Fatalf("GeneratePieces: %v", err)
	}

	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		t.Fatalf("bencode.Marshal: %v", err)
	}

	mi := metainfo.MetaInfo{InfoBytes: infoBytes}
	mi.SetDefaults()
	return mi
}

// startSeeder builds a torrent from sourceFile and seeds it from a real
// torrent.Client with no tracker/DHT (loopback-only test peer). Its Close()
// is registered via t.Cleanup — called here, after sourceFile's t.TempDir()
// was already registered — so the seed file's handle releases before
// TempDir tries to remove it (otherwise Windows reports "file in use").
func startSeeder(t *testing.T, sourceFile string) (*torrent.Client, *torrent.Torrent, metainfo.MetaInfo) {
	t.Helper()

	mi := buildTestTorrent(t, sourceFile)

	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = filepath.Dir(sourceFile)
	cfg.Seed = true
	cfg.NoDHT = true
	cfg.DisablePEX = true
	cfg.ListenPort = 0

	client, err := torrent.NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient (seeder): %v", err)
	}
	t.Cleanup(func() { client.Close() })

	tor, err := client.AddTorrent(&mi)
	if err != nil {
		t.Fatalf("AddTorrent (seeder): %v", err)
	}
	if err := tor.VerifyDataContext(t.Context()); err != nil {
		t.Fatalf("VerifyDataContext (seeder): %v", err)
	}

	return client, tor, mi
}

// newSeedDir returns a temp directory for seed source files, with
// best-effort (error-ignoring) cleanup instead of t.TempDir()'s.
// torrent.Client.Close() doesn't synchronously release its storage
// backend's file handles on Windows even after Client.Closed() fires, which
// otherwise races t.TempDir()'s own cleanup into a hard test failure
// ("The process cannot access the file..."). The download/seed correctness
// itself doesn't depend on how fast the OS reclaims the handle afterward —
// this only relaxes how strictly that unrelated timing gets enforced.
func newSeedDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "nekodl-seed-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
