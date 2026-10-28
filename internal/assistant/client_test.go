package assistant

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestUnicodeChunkingPreservesTail(t *testing.T) {
	input := strings.Repeat("İstanbul bakım 🛠 ", 180) + "SON"
	parts := Split(input)
	if len(parts) < 2 {
		t.Fatal("not split")
	}
	if !strings.HasSuffix(parts[len(parts)-1], "SON") {
		t.Fatal("tail lost")
	}
	for _, p := range parts {
		if len([]rune(p)) > 900 {
			t.Fatal("oversized chunk")
		}
	}
}
func TestEmbeddingResponseValidation(t *testing.T) {
	for _, body := range []string{`{"embeddings":[]}`, `{"embeddings":[[]]}`, `{"embeddings":[[0,0]]}`, `{"embeddings":[[0.5,0.5]]}`} {
		t.Run(body, func(t *testing.T) {
			client := New("http://ollama.test", "embed", "chat")
			client.HTTP = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/api/embed" {
					t.Error(r.URL.Path)
				}
				var in map[string]any
				json.NewDecoder(r.Body).Decode(&in)
				if in["truncate"] != false {
					t.Error("silent truncation enabled")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			_, err := client.embed(context.Background(), "text")
			if (err == nil) != (strings.Contains(body, "0.5")) {
				t.Fatalf("unexpected result %v", err)
			}
		})
	}
}

func TestSuggestionRejectsInventedReferences(t *testing.T) {
	base := Suggestion{Sources: []Source{{ID: "fault-12", Title: "Onaylı çözüm", Content: "Ölçüm kaydı"}}, Citations: []string{}}
	for _, response := range []string{`{"answer":"Uydurma öneri","citations":["fault-999"]}`, `{"answer":"Kaynak yok","citations":[]}`, `not json`, `{"answer":"","citations":["fault-12"]}`} {
		got := finalizeSuggestion(base, response)
		if got.Mode != "sources_only" || len(got.Citations) != 0 || strings.Contains(got.Answer, "Uydurma") {
			t.Fatalf("unverified model output accepted: %+v", got)
		}
		if len(got.Sources) != 1 {
			t.Fatal("fallback lost real source")
		}
	}
	got := finalizeSuggestion(base, `{"answer":"Kayda göre inceleyin","citations":["fault-12","fault-12"]}`)
	if got.Mode != "generated" || len(got.Citations) != 1 || got.Answer != "Kayda göre inceleyin" {
		t.Fatalf("valid citation rejected: %+v", got)
	}
}

func TestGenerationUsesKnownReferenceSchema(t *testing.T) {
	c := New("http://ollama.test", "embed", "chat")
	sources := []Source{{ID: "fault-10", Title: "Çözüm", Content: "DEMO-TLM-03: yayın 10 saniye, uyarı 30 saniye"}}
	request := c.generationRequest("TELEMETRY_INTERVAL_MISMATCH", "Yeni gözlem", sources)
	format := request["format"].(map[string]any)
	properties := format["properties"].(map[string]any)
	item := properties["citations"].(map[string]any)["items"].(map[string]any)
	ids := item["enum"].([]string)
	if len(ids) != 1 || ids[0] != "fault-10" {
		t.Fatal("schema allows references outside retrieved sources")
	}
	prompt := request["prompt"].(string)
	for _, value := range []string{"DEMO-TLM-03", "10 saniye", "30 saniye", "Yeni gözlem"} {
		if !strings.Contains(prompt, value) {
			t.Fatal("source information lost from generation context", value)
		}
	}
}
