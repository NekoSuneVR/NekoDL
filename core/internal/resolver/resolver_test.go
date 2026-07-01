package resolver

import (
	"context"
	"testing"
)

func TestDropboxResolve(t *testing.T) {
	d := Dropbox{}

	if !d.CanResolve("https://www.dropbox.com/s/abc123/file.zip?dl=0") {
		t.Fatal("expected Dropbox to claim a dropbox.com URL")
	}
	if d.CanResolve("https://pixeldrain.com/u/abc123") {
		t.Fatal("Dropbox should not claim a non-dropbox URL")
	}

	res, err := d.Resolve(context.Background(), "https://www.dropbox.com/s/abc123/file.zip?dl=0")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(res.URLs) != 1 {
		t.Fatalf("expected 1 URL, got %d", len(res.URLs))
	}
	want := "https://www.dropbox.com/s/abc123/file.zip?dl=1"
	if res.URLs[0] != want {
		t.Fatalf("got %q, want %q", res.URLs[0], want)
	}
}

func TestDropboxResolveAddsMissingDlParam(t *testing.T) {
	d := Dropbox{}
	res, err := d.Resolve(context.Background(), "https://www.dropbox.com/s/abc123/file.zip")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := "https://www.dropbox.com/s/abc123/file.zip?dl=1"
	if res.URLs[0] != want {
		t.Fatalf("got %q, want %q", res.URLs[0], want)
	}
}

func TestPixeldrainResolve(t *testing.T) {
	p := Pixeldrain{}

	if !p.CanResolve("https://pixeldrain.com/u/abc123") {
		t.Fatal("expected Pixeldrain to claim a pixeldrain.com/u/ URL")
	}
	if p.CanResolve("https://www.dropbox.com/s/abc123/file.zip") {
		t.Fatal("Pixeldrain should not claim a non-pixeldrain URL")
	}

	res, err := p.Resolve(context.Background(), "https://pixeldrain.com/u/abc123")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := "https://pixeldrain.com/api/file/abc123"
	if len(res.URLs) != 1 || res.URLs[0] != want {
		t.Fatalf("got %v, want [%q]", res.URLs, want)
	}
}

func TestRegistryDispatchesToMatchingResolver(t *testing.T) {
	reg := NewRegistry(Dropbox{}, Pixeldrain{})

	res, err := reg.Resolve(context.Background(), "https://pixeldrain.com/u/xyz789")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.URLs[0] != "https://pixeldrain.com/api/file/xyz789" {
		t.Fatalf("got %v", res.URLs)
	}
}

func TestRegistryReturnsErrNotSupportedForUnknownHost(t *testing.T) {
	reg := NewRegistry(Dropbox{}, Pixeldrain{})

	_, err := reg.Resolve(context.Background(), "https://example.com/some/file.zip")
	if err != ErrNotSupported {
		t.Fatalf("expected ErrNotSupported, got %v", err)
	}
}
