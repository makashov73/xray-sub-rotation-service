package sublist

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	dir := t.TempDir()
	content := `# Comment line
abc123 | https://xray1.example.com/sub/abc123 | Server 1

xyz789 | https://xray2.example.com/sub/xyz789 | Server 2
`
	path := filepath.Join(dir, "sublist.md")
	os.WriteFile(path, []byte(content), 0644)

	entries, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}

	if entries[0].SubId != "abc123" {
		t.Errorf("First SubId = %q, want %q", entries[0].SubId, "abc123")
	}
	if entries[0].URL != "https://xray1.example.com/sub/abc123" {
		t.Errorf("First URL = %q", entries[0].URL)
	}
	if entries[0].Name != "Server 1" {
		t.Errorf("First Name = %q, want %q", entries[0].Name, "Server 1")
	}
}

func TestParseInvalidURL(t *testing.T) {
	dir := t.TempDir()
	content := `abc | not-a-valid-url://[bad
`
	path := filepath.Join(dir, "sublist.md")
	os.WriteFile(path, []byte(content), 0644)

	_, err := Parse(path)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}
