package megalink

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// decryptAttributes decrypts MEGA's "at" attribute blob (AES-CBC, zero IV)
// and pulls out the filename. MEGA prefixes the JSON payload with the
// literal string "MEGA" and zero-pads it to a block boundary.
func decryptAttributes(encoded []byte, aesKey []byte) (name string, err error) {
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return "", err
	}
	if len(encoded)%aes.BlockSize != 0 {
		return "", fmt.Errorf("megalink: attribute block length %d is not a multiple of the AES block size", len(encoded))
	}

	plain := make([]byte, len(encoded))
	cipher.NewCBCDecrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(plain, encoded)
	plain = trimTrailingNulls(plain)

	if !strings.HasPrefix(string(plain), "MEGA") {
		return "", fmt.Errorf("megalink: decrypted attributes are missing the expected MEGA prefix (wrong key?)")
	}

	var attrs struct {
		Name string `json:"n"`
	}
	if err := json.Unmarshal(plain[4:], &attrs); err != nil {
		return "", fmt.Errorf("megalink: parsing decrypted attributes: %w", err)
	}
	return attrs.Name, nil
}

func trimTrailingNulls(b []byte) []byte {
	for len(b) > 0 && b[len(b)-1] == 0 {
		b = b[:len(b)-1]
	}
	return b
}

// decryptingReader wraps r so reads from it yield MEGA file-data plaintext.
func decryptingReader(r io.Reader, fk FileKey) (io.Reader, error) {
	block, err := aes.NewCipher(fk.AESKey)
	if err != nil {
		return nil, err
	}
	stream := cipher.NewCTR(block, fk.IV)
	return &cipher.StreamReader{S: stream, R: r}, nil
}
