package megalink

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
)

// defaultAPIBaseURL is MEGA's classic API endpoint, used by long-standing
// community client implementations for the "g" (get) action.
const defaultAPIBaseURL = "https://g.api.mega.co.nz/cs"

type fileInfo struct {
	DownloadURL string `json:"g"`
	Size        int64  `json:"s"`
	Attributes  string `json:"at"`
}

// fetchFileInfo calls MEGA's "g" API action for a public file handle:
// POST {baseURL}?id=<random> with body [{"a":"g","g":1,"p":"<handle>"}],
// response is a JSON array whose first element is either the file info
// object or a bare negative integer error code.
func fetchFileInfo(ctx context.Context, client *http.Client, baseURL, handle string) (fileInfo, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if baseURL == "" {
		baseURL = defaultAPIBaseURL
	}

	body, err := json.Marshal([]map[string]any{{"a": "g", "g": 1, "p": handle}})
	if err != nil {
		return fileInfo{}, err
	}

	reqURL := fmt.Sprintf("%s?id=%d", baseURL, rand.Intn(0xFFFFFFF))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return fileInfo{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fileInfo{}, err
	}
	defer resp.Body.Close()

	var results []json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return fileInfo{}, fmt.Errorf("megalink: decoding API response: %w", err)
	}
	if len(results) == 0 {
		return fileInfo{}, fmt.Errorf("megalink: empty API response")
	}

	var errCode int
	if err := json.Unmarshal(results[0], &errCode); err == nil {
		return fileInfo{}, fmt.Errorf("megalink: MEGA API returned error code %d", errCode)
	}

	var info fileInfo
	if err := json.Unmarshal(results[0], &info); err != nil {
		return fileInfo{}, fmt.Errorf("megalink: parsing file info: %w", err)
	}
	return info, nil
}
