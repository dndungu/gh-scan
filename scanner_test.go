package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInventoryAndReadmeFiles(t *testing.T) {
	docs := []RepoDoc{
		{Owner: "example", Name: "first", URL: "https://github.com/example/first", Description: "First project", Readme: "# First\nSome details.\n"},
		{Owner: "example", Name: "second", Description: "Second project", Err: "no README or fetch failed"},
	}
	root := filepath.Join(t.TempDir(), "outputs")
	if err := WriteReadmes(docs, root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "example", "first", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != docs[0].Readme {
		t.Fatalf("README content = %q", data)
	}
	if _, err := os.Stat(filepath.Join(root, "example", "second", "README.md")); !os.IsNotExist(err) {
		t.Fatalf("missing README file: expected not to exist, got %v", err)
	}
	md := RenderMarkdown(docs)
	for _, want := range []string{"First project", "Second project", "https://github.com/example/first"} {
		if !strings.Contains(md, want) {
			t.Errorf("inventory missing %q", want)
		}
	}
	for _, unwanted := range []string{"# First", "Some details.", "no README or fetch failed"} {
		if strings.Contains(md, unwanted) {
			t.Errorf("inventory contains %q", unwanted)
		}
	}
}

func TestWriteReadmesRejectsUnsafePath(t *testing.T) {
	docs := []RepoDoc{{Owner: "..", Name: "repo", Readme: "content"}}
	if err := WriteReadmes(docs, t.TempDir()); err == nil {
		t.Fatal("expected invalid repository path error")
	}
}
