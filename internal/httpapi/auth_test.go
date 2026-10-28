package httpapi

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestJWTIntegrityAndExpiry(t *testing.T) {
	secret := strings.Repeat("a", 32)
	c := Claims{12, "admin", time.Now().Add(time.Minute).Unix()}
	token := sign(c, secret)
	got, err := verify(token, secret)
	if err != nil || got != c {
		t.Fatalf("valid token rejected: %v", err)
	}
	parts := strings.Split(token, ".")
	parts[1] = base64.RawURLEncoding.EncodeToString([]byte(`{"sub":1,"role":"admin","exp":9999999999}`))
	for name, value := range map[string]string{"tampered": strings.Join(parts, "."), "expired": sign(Claims{1, "admin", 1}, secret), "malformed": "abc", "none": "eyJhbGciOiJub25lIn0.e30."} {
		t.Run(name, func(t *testing.T) {
			if _, err := verify(value, secret); err == nil {
				t.Fatal("unsafe token accepted")
			}
		})
	}
	if _, err = verify(token, "wrong-secret"); err == nil {
		t.Fatal("wrong signing key accepted")
	}
}
func TestDecodeRejectsUnexpectedInput(t *testing.T) {
	for _, body := range []string{`{"known":"ok","extra":1}`, `{"known":"ok"} {}`, `{`, strings.Repeat("x", (1<<20)+1)} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		w := httptest.NewRecorder()
		var in struct {
			Known string `json:"known"`
		}
		if decode(w, r, &in) {
			t.Fatal("unsafe body accepted")
		}
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
}

func TestLoginRateLimit(t *testing.T) {
	handler := limitLogin(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	for i := 0; i < 11; i++ {
		r := httptest.NewRequest("POST", "/api/auth/login", nil)
		r.RemoteAddr = "192.0.2.1:1234"
		w := httptest.NewRecorder()
		handler(w, r)
		want := 204
		if i == 10 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("attempt %d: %d", i, w.Code)
		}
	}
	r := httptest.NewRequest("POST", "/api/auth/login", nil)
	r.RemoteAddr = "192.0.2.2:1234"
	w := httptest.NewRecorder()
	handler(w, r)
	if w.Code != 204 {
		t.Fatal("unrelated client blocked")
	}
}
