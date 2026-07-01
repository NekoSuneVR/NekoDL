package resolver

import (
	"context"
	"testing"
)

func TestGoogleDriveResolveFileDLink(t *testing.T) {
	g := GoogleDrive{}
	rawURL := "https://drive.google.com/file/d/1AbCdEfGhIjKlMnOp/view?usp=sharing"

	if !g.CanResolve(rawURL) {
		t.Fatal("expected GoogleDrive to claim a /file/d/ URL")
	}

	res, err := g.Resolve(context.Background(), rawURL)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := "https://drive.google.com/uc?export=download&id=1AbCdEfGhIjKlMnOp"
	if len(res.URLs) != 1 || res.URLs[0] != want {
		t.Fatalf("got %v, want [%q]", res.URLs, want)
	}
}

func TestGoogleDriveResolveOpenIDLink(t *testing.T) {
	g := GoogleDrive{}
	rawURL := "https://drive.google.com/open?id=1AbCdEfGhIjKlMnOp"

	if !g.CanResolve(rawURL) {
		t.Fatal("expected GoogleDrive to claim an ?id= URL")
	}

	res, err := g.Resolve(context.Background(), rawURL)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := "https://drive.google.com/uc?export=download&id=1AbCdEfGhIjKlMnOp"
	if res.URLs[0] != want {
		t.Fatalf("got %q, want %q", res.URLs[0], want)
	}
}

func TestGoogleDriveCanResolveRejectsOtherHosts(t *testing.T) {
	g := GoogleDrive{}
	if g.CanResolve("https://docs.google.com/document/d/xyz") {
		t.Fatal("expected GoogleDrive to reject a docs.google.com URL")
	}
	if g.CanResolve("https://drive.google.com/drive/folders/xyz") {
		t.Fatal("expected GoogleDrive to reject a folder URL with no id")
	}
}
