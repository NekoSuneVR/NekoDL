package resolver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMediafireCanResolve(t *testing.T) {
	m := Mediafire{}
	if !m.CanResolve("https://www.mediafire.com/file/abc123/example.zip/file") {
		t.Fatal("expected Mediafire to claim a mediafire.com URL")
	}
	if m.CanResolve("https://www.dropbox.com/s/abc/file.zip") {
		t.Fatal("Mediafire should not claim a non-mediafire URL")
	}
}

// TestExtractMediafireDownloadURL checks the HTML-scraping regex against a
// constructed page matching Mediafire's documented downloadButton pattern.
// Not live-verified against a real share page — see mediafire.go for why.
func TestExtractMediafireDownloadURL(t *testing.T) {
	cases := []struct {
		name string
		html string
		want string
		ok   bool
	}{
		{
			name: "href after id",
			html: `<div><a aria-label="Download file" id="downloadButton" href="https://download2390.mediafire.com/abc/def/file.zip" class="input popsok">Download</a></div>`,
			want: "https://download2390.mediafire.com/abc/def/file.zip",
			ok:   true,
		},
		{
			name: "href before id",
			html: `<a href="https://download123.mediafire.com/x/y/file.zip" id="downloadButton">Download</a>`,
			want: "https://download123.mediafire.com/x/y/file.zip",
			ok:   true,
		},
		{
			name: "escaped ampersand in URL",
			html: `<a id="downloadButton" href="https://download1.mediafire.com/f?x=1&amp;y=2">Download</a>`,
			want: "https://download1.mediafire.com/f?x=1&y=2",
			ok:   true,
		},
		{
			name: "no download button present",
			html: `<html><body>File not found</body></html>`,
			ok:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := extractMediafireDownloadURL(tc.html)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMediafireResolveAgainstMockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><a id="downloadButton" href="https://download99.mediafire.com/z/z/mockfile.bin">Download</a></body></html>`))
	}))
	defer srv.Close()

	m := Mediafire{}
	res, err := m.Resolve(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := "https://download99.mediafire.com/z/z/mockfile.bin"
	if len(res.URLs) != 1 || res.URLs[0] != want {
		t.Fatalf("got %v, want [%q]", res.URLs, want)
	}
}

func TestMediafireResolveFailsWithoutDownloadButton(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>Invalid or deleted file.</body></html>`))
	}))
	defer srv.Close()

	m := Mediafire{}
	if _, err := m.Resolve(context.Background(), srv.URL); err == nil {
		t.Fatal("expected an error when no download button is present")
	}
}
