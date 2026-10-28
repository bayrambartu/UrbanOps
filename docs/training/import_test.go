package training

import (
	"strings"
	"testing"
)

func TestSourceBundleHasTraceableDocuments(t *testing.T) {
	documents, err := Documents()
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) == 0 {
		t.Fatal("no documents")
	}
	seen := map[string]bool{}
	for _, d := range documents {
		content, err := assets.ReadFile(d.File)
		if err != nil {
			t.Fatal(err)
		}
		if seen[d.Title] {
			t.Fatal("duplicate title")
		}
		seen[d.Title] = true
		if !strings.Contains(string(content), "URL: "+d.URL) || !strings.HasPrefix(d.URL, "https://") {
			t.Fatal("missing source trace", d.Title)
		}
		if len(content) > 100000 || len([]rune(string(content))) < 30 {
			t.Fatal("document violates upload bounds")
		}
	}
}
