package megalink

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestParseLinkModernFormat(t *testing.T) {
	handle, key, err := ParseLink("https://mega.nz/file/AbCdEfGh#some-key_value")
	if err != nil {
		t.Fatalf("ParseLink: %v", err)
	}
	if handle != "AbCdEfGh" || key != "some-key_value" {
		t.Fatalf("got handle=%q key=%q", handle, key)
	}
}

func TestParseLinkLegacyFormat(t *testing.T) {
	handle, key, err := ParseLink("https://mega.nz/#!AbCdEfGh!some-key_value")
	if err != nil {
		t.Fatalf("ParseLink: %v", err)
	}
	if handle != "AbCdEfGh" || key != "some-key_value" {
		t.Fatalf("got handle=%q key=%q", handle, key)
	}
}

func TestParseLinkRejectsNonMegaHost(t *testing.T) {
	if _, _, err := ParseLink("https://example.com/file/AbCdEfGh#key"); err == nil {
		t.Fatal("expected an error for a non-mega.nz host")
	}
}

func TestCanResolve(t *testing.T) {
	if !CanResolve("https://mega.nz/file/AbCdEfGh#key") {
		t.Fatal("expected CanResolve to accept a well-formed link")
	}
	if CanResolve("https://example.com/nope") {
		t.Fatal("expected CanResolve to reject a non-mega URL")
	}
}

func TestBytesToA32AndBackRoundTrip(t *testing.T) {
	original := []byte("0123456789ABCDEF") // exactly 16 bytes, no padding needed
	words := bytesToA32(original)
	if len(words) != 4 {
		t.Fatalf("expected 4 words, got %d", len(words))
	}
	back := a32ToBytes(words)
	if !bytes.Equal(back, original) {
		t.Fatalf("round trip mismatch: got %q want %q", back, original)
	}
}

// TestDeriveFileKeyMatchesFormula constructs a known 8-word (32-byte) key
// and checks the AES key / IV / MAC come out exactly per the documented
// formula: AES key = XOR of the two 4-word halves; IV = words[4],words[5]
// followed by 8 zero bytes; MAC = words[6],words[7].
func TestDeriveFileKeyMatchesFormula(t *testing.T) {
	words := []uint32{
		0x11111111, 0x22222222, 0x33333333, 0x44444444, // first half
		0xAAAAAAAA, 0xBBBBBBBB, 0xCCCCCCCC, 0xDDDDDDDD, // second half
	}
	raw := a32ToBytes(words)
	keyB64 := base64.RawURLEncoding.EncodeToString(raw)

	fk, err := deriveFileKey(keyB64)
	if err != nil {
		t.Fatalf("deriveFileKey: %v", err)
	}

	wantAESKey := a32ToBytes([]uint32{
		0x11111111 ^ 0xAAAAAAAA,
		0x22222222 ^ 0xBBBBBBBB,
		0x33333333 ^ 0xCCCCCCCC,
		0x44444444 ^ 0xDDDDDDDD,
	})
	if !bytes.Equal(fk.AESKey, wantAESKey) {
		t.Fatalf("AES key mismatch: got %x want %x", fk.AESKey, wantAESKey)
	}

	wantIV := make([]byte, 16)
	binary.BigEndian.PutUint32(wantIV[0:4], 0xAAAAAAAA)
	binary.BigEndian.PutUint32(wantIV[4:8], 0xBBBBBBBB)
	if !bytes.Equal(fk.IV, wantIV) {
		t.Fatalf("IV mismatch: got %x want %x", fk.IV, wantIV)
	}

	if fk.MetaMAC != [2]uint32{0xCCCCCCCC, 0xDDDDDDDD} {
		t.Fatalf("MAC mismatch: got %x", fk.MetaMAC)
	}
}

// TestCTRDecryptRoundTripAgainstStdlib is the key crypto-correctness check:
// it encrypts known plaintext using Go's own crypto/cipher.NewCTR (acting
// as an independent "reference encryptor"), then decrypts it with this
// package's decryptingReader and confirms the plaintext comes back exactly.
// This validates the IV/counter construction, not just self-consistency.
func TestCTRDecryptRoundTripAgainstStdlib(t *testing.T) {
	fk := FileKey{
		AESKey: bytes.Repeat([]byte{0x42}, 16),
		IV:     append(bytes.Repeat([]byte{0x01}, 8), make([]byte, 8)...),
	}

	plaintext := bytes.Repeat([]byte("The quick brown fox jumps. "), 200) // multi-block, non-aligned length

	block, err := aes.NewCipher(fk.AESKey)
	if err != nil {
		t.Fatalf("aes.NewCipher: %v", err)
	}
	encryptStream := cipher.NewCTR(block, fk.IV)
	ciphertext := make([]byte, len(plaintext))
	encryptStream.XORKeyStream(ciphertext, plaintext)

	reader, err := decryptingReader(bytes.NewReader(ciphertext), fk)
	if err != nil {
		t.Fatalf("decryptingReader: %v", err)
	}
	got := make([]byte, len(plaintext))
	if _, err := io.ReadFull(reader, got); err != nil {
		t.Fatalf("reading decrypted stream: %v", err)
	}

	if !bytes.Equal(got, plaintext) {
		t.Fatal("decrypted plaintext does not match the original")
	}
}

// TestDecryptAttributesRoundTrip encrypts a MEGA-style attribute blob with
// Go's own stdlib AES-CBC (independent of this package's decrypt path) and
// confirms decryptAttributes recovers the filename.
func TestDecryptAttributesRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{0x07}, 16)

	payload := []byte(`MEGA{"n":"hello-world.txt"}`)
	for len(payload)%aes.BlockSize != 0 {
		payload = append(payload, 0)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("aes.NewCipher: %v", err)
	}
	encrypted := make([]byte, len(payload))
	cipher.NewCBCEncrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(encrypted, payload)

	name, err := decryptAttributes(encrypted, key)
	if err != nil {
		t.Fatalf("decryptAttributes: %v", err)
	}
	if name != "hello-world.txt" {
		t.Fatalf("got name %q", name)
	}
}

func TestDecryptAttributesRejectsWrongKey(t *testing.T) {
	rightKey := bytes.Repeat([]byte{0x07}, 16)
	wrongKey := bytes.Repeat([]byte{0x08}, 16)

	payload := []byte(`MEGA{"n":"secret.txt"}`)
	for len(payload)%aes.BlockSize != 0 {
		payload = append(payload, 0)
	}
	block, _ := aes.NewCipher(rightKey)
	encrypted := make([]byte, len(payload))
	cipher.NewCBCEncrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(encrypted, payload)

	if _, err := decryptAttributes(encrypted, wrongKey); err == nil {
		t.Fatal("expected an error decrypting with the wrong key")
	}
}

func TestFetchFileInfoParsesErrorCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("[-9]")) // MEGA's ENOENT
	}))
	defer srv.Close()

	_, err := fetchFileInfo(context.Background(), nil, srv.URL, "somehandle")
	if err == nil {
		t.Fatal("expected an error for a MEGA error-code response")
	}
}

func TestFullDownloadPipeline(t *testing.T) {
	// Build a real file key the same way deriveFileKey would produce one,
	// then independently AES-CTR-encrypt real file bytes and AES-CBC-encrypt
	// real attributes with it — so the test's "server side" is genuinely
	// independent of the package's own decrypt code.
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

	plaintext := bytes.Repeat([]byte("NekoDL end-to-end mega test payload. "), 500)

	block, err := aes.NewCipher(fk.AESKey)
	if err != nil {
		t.Fatalf("aes.NewCipher: %v", err)
	}
	ciphertext := make([]byte, len(plaintext))
	cipher.NewCTR(block, fk.IV).XORKeyStream(ciphertext, plaintext)

	attrPayload := []byte(`MEGA{"n":"pipeline-test.bin"}` + "\x00\x00")
	for len(attrPayload)%aes.BlockSize != 0 {
		attrPayload = append(attrPayload, 0)
	}
	encryptedAttrs := make([]byte, len(attrPayload))
	cipher.NewCBCEncrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(encryptedAttrs, attrPayload)
	attrsB64 := base64.RawURLEncoding.EncodeToString(encryptedAttrs)

	ciphertextSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(ciphertext)
	}))
	defer ciphertextSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := []map[string]any{{
			"g":  ciphertextSrv.URL,
			"s":  len(plaintext),
			"at": attrsB64,
		}}
		data, _ := json.Marshal(resp)
		_, _ = w.Write(data)
	}))
	defer apiSrv.Close()

	d := &Downloader{APIBaseURL: apiSrv.URL}
	dest := filepath.Join(t.TempDir(), "out.bin")

	meta, err := d.Download(context.Background(), fmt.Sprintf("https://mega.nz/file/testhandle#%s", keyB64), dest)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if meta.Name != "pipeline-test.bin" {
		t.Fatalf("got name %q", meta.Name)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatal("decrypted output does not match the original plaintext")
	}
}
