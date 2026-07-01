package megalink

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
)

// FileKey is the derived AES key and CTR seed for one MEGA file.
type FileKey struct {
	AESKey  []byte // 16 bytes
	IV      []byte // 16 bytes — the initial CTR counter block
	MetaMAC [2]uint32
}

// deriveFileKey splits MEGA's 8-word (32-byte) node key into an AES-128 key
// and CTR parameters. Per the node key format: the AES key is the XOR of
// the key's two 4-word halves; words 4-5 seed the high 64 bits of the
// initial CTR counter (the low 64 bits start at zero and increment per
// block, which is exactly what crypto/cipher.NewCTR already does with a
// 16-byte starting IV); words 6-7 are an integrity MAC this package doesn't
// currently verify (see TODO.md).
func deriveFileKey(keyB64 string) (FileKey, error) {
	raw, err := base64.RawURLEncoding.DecodeString(keyB64)
	if err != nil {
		return FileKey{}, fmt.Errorf("megalink: decoding key: %w", err)
	}

	words := bytesToA32(raw)
	if len(words) != 8 {
		return FileKey{}, fmt.Errorf("megalink: expected an 8-word (32-byte) file key, got %d words", len(words))
	}

	k := make([]uint32, 4)
	for i := 0; i < 4; i++ {
		k[i] = words[i] ^ words[i+4]
	}

	iv := make([]byte, 16)
	binary.BigEndian.PutUint32(iv[0:4], words[4])
	binary.BigEndian.PutUint32(iv[4:8], words[5])
	// iv[8:16] stays zero: the low 64 bits of the initial CTR counter.

	return FileKey{
		AESKey:  a32ToBytes(k),
		IV:      iv,
		MetaMAC: [2]uint32{words[6], words[7]},
	}, nil
}

// bytesToA32 unpacks bytes into big-endian 32-bit words, zero-padding to a
// multiple of 4 bytes first.
func bytesToA32(b []byte) []uint32 {
	if rem := len(b) % 4; rem != 0 {
		padded := make([]byte, len(b)+(4-rem))
		copy(padded, b)
		b = padded
	}
	words := make([]uint32, len(b)/4)
	for i := range words {
		words[i] = binary.BigEndian.Uint32(b[i*4 : i*4+4])
	}
	return words
}

func a32ToBytes(words []uint32) []byte {
	b := make([]byte, len(words)*4)
	for i, w := range words {
		binary.BigEndian.PutUint32(b[i*4:i*4+4], w)
	}
	return b
}
