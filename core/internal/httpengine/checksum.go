package httpengine

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"
)

// Checksum verifies a completed download's contents against a known digest.
type Checksum struct {
	Algo     string // "md5", "sha1", or "sha256"
	Expected string // hex-encoded
}

func verifyChecksum(path string, c Checksum) error {
	var h hash.Hash
	switch strings.ToLower(c.Algo) {
	case "md5":
		h = md5.New()
	case "sha1":
		h = sha1.New()
	case "sha256":
		h = sha256.New()
	default:
		return fmt.Errorf("httpengine: unsupported checksum algorithm %q", c.Algo)
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, c.Expected) {
		return fmt.Errorf("httpengine: checksum mismatch: expected %s, got %s", c.Expected, actual)
	}
	return nil
}
