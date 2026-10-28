package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"urbanops/internal/assistant"
	"urbanops/internal/domain"
	"urbanops/internal/httpapi"
	"urbanops/internal/platform"
	"urbanops/migrations"
)

type offlineModelTransport struct{}

func (offlineModelTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("model offline in integration scenario")
}

type historicalGenerationTransport struct {
	SourceID string
	Paths    []string
}

func (t *historicalGenerationTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.Paths = append(t.Paths, r.URL.Path)
	if r.URL.Path != "/api/generate" {
		return nil, fmt.Errorf("unexpected embedding call with no indexed documents")
	}
	answer, _ := json.Marshal(map[string]any{"answer": "Onaylı geçmiş çözümdeki doğrulama adımlarını inceleyin.", "citations": []string{t.SourceID}})
	body, _ := json.Marshal(map[string]string{"response": string(answer)})
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
}

func TestDurableIngestionAndWorkOrderLifecycle(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	rabbit := os.Getenv("TEST_RABBITMQ_URL")
	if url == "" || rabbit == "" {
		t.Skip("TEST_DATABASE_URL and TEST_RABBITMQ_URL required; isolated test services only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = migrations.Apply(ctx, db); err != nil {
		t.Fatal(err)
	}
	b, err := platform.OpenBroker(rabbit)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	nonce := fmt.Sprintf("%d", time.Now().UnixNano())
	device := "test-" + nonce
	email := "test-" + nonce + "@local"
	password := "integration-password"
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	var admin, tech, tech2 int64
	if _, err = db.Exec(ctx, `INSERT INTO devices(id,name,kind,location) VALUES($1,'Integration','simulator','Test')`, device); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO users(email,name,password_hash,role) VALUES($1,'Admin',$2,'admin') RETURNING id`, email, string(hash)).Scan(&admin); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO users(email,name,password_hash,role) VALUES($1,'Tech',$2,'technician') RETURNING id`, "tech-"+email, string(hash)).Scan(&tech); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO users(email,name,password_hash,role) VALUES($1,'Tech 2',$2,'technician') RETURNING id`, "other-"+email, string(hash)).Scan(&tech2); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean := context.Background()
		db.Exec(clean, `DELETE FROM audit_log WHERE work_order_id IN(SELECT w.id FROM work_orders w JOIN faults f ON f.id=w.fault_id WHERE f.device_id=$1)`, device)
		db.Exec(clean, `DELETE FROM work_orders WHERE fault_id IN(SELECT id FROM faults WHERE device_id=$1)`, device)
		db.Exec(clean, `DELETE FROM events WHERE fault_id IN(SELECT id FROM faults WHERE device_id=$1)`, device)
		db.Exec(clean, `DELETE FROM faults WHERE device_id=$1`, device)
		db.Exec(clean, `DELETE FROM devices WHERE id=$1`, device)
		db.Exec(clean, `DELETE FROM users WHERE id IN($1,$2,$3)`, admin, tech, tech2)
		db.Exec(clean, `DELETE FROM chunks WHERE document_id IN(SELECT id FROM documents WHERE title=$1)`, "test-doc-"+nonce)
		db.Exec(clean, `DELETE FROM documents WHERE title=$1`, "test-doc-"+nonce)
	}()
	offlineAI := assistant.New("http://ollama.test", "test-embed", "test-chat")
	offlineAI.HTTP = &http.Client{Transport: offlineModelTransport{}}
	srv := (&httpapi.Server{DB: db, Broker: b, AI: offlineAI, Secret: strings.Repeat("s", 32), IngestToken: strings.Repeat("i", 32)}).Routes()
	request := func(path, method, token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		return w
	}
	login := request("/api/auth/login", "POST", "", fmt.Sprintf(`{"email":%q,"password":%q}`, email, password))
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	var session struct {
		Token string `json:"token"`
	}
	json.Unmarshal(login.Body.Bytes(), &session)
	e := domain.Event{EventID: "event-" + nonce, DeviceID: device, FaultCode: "TEST_TIMEOUT", Severity: "high", Location: "Test", Description: "Integration event", Timestamp: time.Now()}
	body, _ := json.Marshal(e)
	if w := request("/api/events", "POST", "wrong", string(body)); w.Code != http.StatusUnauthorized {
		t.Fatal("unauthorized ingestion")
	}
	if w := request("/api/events", "POST", strings.Repeat("i", 32), string(body)); w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	// Obtain the actual broker delivery and process it using the worker's transaction.
	delivery, ok, err := b.Ch.Get(platform.Queue, false)
	if err != nil || !ok {
		t.Fatalf("broker delivery missing: %v", err)
	}
	if delivery.MessageId != e.EventID {
		delivery.Nack(false, true)
		t.Fatal("test RabbitMQ must be isolated and empty")
	}
	var received domain.Event
	if err = json.Unmarshal(delivery.Body, &received); err != nil {
		t.Fatal(err)
	}
	if err = platform.ApplyEvent(ctx, db, received); err != nil {
		t.Fatal(err)
	}
	delivery.Ack(false)
	// Concurrent redeliveries must not increment occurrences or create extra work.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := platform.ApplyEvent(ctx, db, e); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	e.EventID += "-repeat"
	if err = platform.ApplyEvent(ctx, db, e); err != nil {
		t.Fatal(err)
	}
	var count int
	var order int64
	if err = db.QueryRow(ctx, `SELECT f.occurrences,w.id FROM faults f JOIN work_orders w ON w.fault_id=f.id WHERE device_id=$1`, device).Scan(&count, &order); err != nil || count != 2 {
		t.Fatalf("dedup failed: %d %v", count, err)
	}
	path := fmt.Sprintf("/api/work-orders/%d", order)
	if w := request(path, "PATCH", session.Token, `{"status":"closed"}`); w.Code != 409 {
		t.Fatal("invalid transition accepted")
	}

	loginAs := func(address string) string {
		t.Helper()
		w := request("/api/auth/login", "POST", "", fmt.Sprintf(`{"email":%q,"password":%q}`, address, password))
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var out struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out.Token
	}
	techToken := loginAs("tech-" + email)
	otherToken := loginAs("other-" + email)
	expect := func(token, body string, status int) {
		t.Helper()
		if w := request(path, "PATCH", token, body); w.Code != status {
			t.Fatalf("%s: got %d want %d: %s", body, w.Code, status, w.Body.String())
		}
	}
	expect(session.Token, fmt.Sprintf(`{"status":"assigned","assignee_id":%d}`, tech), 200)
	expect(session.Token, `{"status":"in_progress"}`, 403)
	expect(otherToken, `{"status":"in_progress"}`, 404)
	if w := request(path+"/history", "GET", otherToken, ""); w.Code != 404 {
		t.Fatal("other technician read history")
	}
	if w := request("/api/reports", "GET", techToken, ""); w.Code != 403 {
		t.Fatal("technician read management reports")
	}
	expect(techToken, `{"status":"in_progress"}`, 200)
	expect(techToken, `{"status":"on_hold"}`, 422)
	expect(techToken, `{"status":"on_hold","note":"Parça bekleniyor"}`, 200)
	expect(techToken, `{"action":"request_reassignment","note":"Vardiya değişimi"}`, 200)
	expect(techToken, `{"action":"request_reassignment","note":"Tekrar"}`, 409)
	var hold string
	var requested bool
	if err = db.QueryRow(ctx, `SELECT hold_reason,reassignment_requested FROM work_orders WHERE id=$1`, order).Scan(&hold, &requested); err != nil || hold != "Parça bekleniyor" || !requested {
		t.Fatal("request lost hold state", err)
	}
	expect(session.Token, fmt.Sprintf(`{"action":"reassign","assignee_id":%d,"note":"Yeni vardiya"}`, tech2), 200)
	expect(techToken, `{"status":"in_progress"}`, 404)
	// Reassigned work must disappear from the previous technician's HTTP lists.
	for _, endpoint := range []string{"/api/work-orders", "/api/faults"} {
		w := request(endpoint, "GET", techToken, "")
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var list []map[string]any
		if err = json.Unmarshal(w.Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
		for _, item := range list {
			if item["device_id"] == device {
				t.Fatal("old technician still sees transferred work")
			}
		}
	}
	expect(otherToken, `{"status":"in_progress"}`, 200)
	expect(otherToken, `{"status":"resolved","note":"Bağlantı geri geldi"}`, 200)
	expect(otherToken, `{"status":"closed"}`, 403)
	expect(session.Token, `{"action":"return","note":"Bağlantı kararlılığı kontrol edilsin"}`, 200)
	expect(otherToken, `{"status":"in_progress"}`, 200)
	expect(otherToken, `{"status":"resolved","note":"Kararlılık kontrol edildi"}`, 200)
	// A new event after resolution creates another open fault. Returning the old
	// solution must fail cleanly instead of violating the partial unique index.
	e.EventID += "-new"
	if err = platform.ApplyEvent(ctx, db, e); err != nil {
		t.Fatal(err)
	}
	expect(session.Token, `{"action":"return","note":"Tekrar kontrol"}`, 409)
	var newFaultID, newOrderID int64
	if err = db.QueryRow(ctx, `SELECT f.id,w.id FROM faults f JOIN work_orders w ON w.fault_id=f.id WHERE f.device_id=$1 AND f.status='open'`, device).Scan(&newFaultID, &newOrderID); err != nil {
		t.Fatal(err)
	}
	similarPath := fmt.Sprintf("/api/faults/%d/similar", newFaultID)
	before := request(similarPath, "GET", session.Token, "")
	if before.Code != 200 || strings.TrimSpace(before.Body.String()) != "[]" {
		t.Fatal("unapproved solution leaked into shared knowledge", before.Body.String())
	}
	expect(session.Token, `{"status":"closed"}`, 200)
	newPath := fmt.Sprintf("/api/work-orders/%d", newOrderID)
	if w := request(newPath, "PATCH", session.Token, fmt.Sprintf(`{"status":"assigned","assignee_id":%d}`, tech2)); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	after := request(similarPath, "GET", otherToken, "")
	var past []assistant.HistoricalSolution
	if err = json.Unmarshal(after.Body.Bytes(), &past); after.Code != 200 || err != nil || len(past) != 1 || past[0].WorkOrderID != order || past[0].ApprovedBy == "" {
		t.Fatal("approved solution unavailable to new assignee", after.Body.String(), err)
	}
	if w := request(similarPath, "GET", techToken, ""); w.Code != 404 {
		t.Fatal("non-owner read similar-case endpoint")
	}
	docBody := fmt.Sprintf(`{"title":%q,"content":"TEST_TIMEOUT: model kapalıyken de okunabilen teknik eğitim kaynağı."}`, "test-doc-"+nonce)
	saved := request("/api/documents", "POST", session.Token, docBody)
	var doc struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}
	if err = json.Unmarshal(saved.Body.Bytes(), &doc); saved.Code != 202 || err != nil || doc.ID == 0 || doc.Status != "pending" {
		t.Fatal("offline document save failed", saved.Body.String(), err)
	}
	again := request("/api/documents", "POST", session.Token, docBody)
	var duplicate struct {
		ID int64 `json:"id"`
	}
	json.Unmarshal(again.Body.Bytes(), &duplicate)
	if again.Code != 202 || duplicate.ID != doc.ID {
		t.Fatal("duplicate document created")
	}
	if w := request(fmt.Sprintf("/api/documents/%d", doc.ID), "GET", otherToken, ""); w.Code != 200 || !strings.Contains(w.Body.String(), "TEST_TIMEOUT") {
		t.Fatal("saved document unreadable")
	}
	if w := request(fmt.Sprintf("/api/documents/%d/index", doc.ID), "POST", session.Token, "{}"); w.Code != 503 {
		t.Fatal("offline index unexpectedly succeeded")
	}
	suggestion := request(fmt.Sprintf("/api/faults/%d/suggestions", newFaultID), "POST", otherToken, "{}")
	var support assistant.Suggestion
	if err = json.Unmarshal(suggestion.Body.Bytes(), &support); suggestion.Code != 200 || err != nil || support.Mode != "sources_only" || len(support.Sources) != 2 || len(support.Citations) != 0 {
		t.Fatal("offline source fallback failed", suggestion.Body.String(), err)
	}
	// Historical knowledge must work without loading an embedding model when
	// there are no indexed documents for that embedding model.
	transport := &historicalGenerationTransport{SourceID: fmt.Sprintf("fault-%d", past[0].FaultID)}
	offlineAI.HTTP = &http.Client{Transport: transport}
	historical := request(fmt.Sprintf("/api/faults/%d/suggestions", newFaultID), "POST", otherToken, "{}")
	var generated assistant.Suggestion
	if err = json.Unmarshal(historical.Body.Bytes(), &generated); historical.Code != 200 || err != nil || generated.Mode != "generated" || len(generated.Citations) != 1 || generated.Citations[0] != transport.SourceID {
		t.Fatal("historical generation incorrectly depends on embeddings", historical.Body.String(), err)
	}
	if len(transport.Paths) != 1 || transport.Paths[0] != "/api/generate" {
		t.Fatal("unnecessary model call", transport.Paths)
	}
	var auditCount int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE work_order_id=$1`, order).Scan(&auditCount); err != nil || auditCount != 11 {
		t.Fatalf("audit: %d %v", auditCount, err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM faults WHERE device_id=$1`, device).Scan(&count); err != nil || count != 2 {
		t.Fatal("new fault not created after resolution")
	}
}
