package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"urbanops/internal/domain"
	"urbanops/internal/httpapi"
	"urbanops/internal/platform"
	"urbanops/migrations"
)

func TestManualReportsAndProvenance(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL required; isolated test database only")
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
	nonce := fmt.Sprintf("%d", time.Now().UnixNano())
	device := "manual-test-" + nonce
	if _, err = db.Exec(ctx, `INSERT INTO devices(id,name,kind,location) VALUES($1,'Manual test','test','Kadıköy')`, device); err != nil {
		t.Fatal(err)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("test-password-123"), bcrypt.MinCost)
	actors := map[string]int64{}
	for key, role := range map[string]string{"admin": "admin", "operator": "operator", "tech": "technician", "other": "technician"} {
		var id int64
		if err = db.QueryRow(ctx, `INSERT INTO users(email,name,password_hash,role) VALUES($1,$2,$3,$4) RETURNING id`, key+nonce+"@local", key, string(hash), role).Scan(&id); err != nil {
			t.Fatal(err)
		}
		actors[key] = id
	}
	defer func() {
		clean := context.Background()
		db.Exec(clean, `DELETE FROM audit_log WHERE work_order_id IN(SELECT w.id FROM work_orders w JOIN faults f ON f.id=w.fault_id WHERE f.device_id=$1)`, device)
		db.Exec(clean, `DELETE FROM events WHERE fault_id IN(SELECT id FROM faults WHERE device_id=$1)`, device)
		db.Exec(clean, `DELETE FROM work_orders WHERE fault_id IN(SELECT id FROM faults WHERE device_id=$1)`, device)
		db.Exec(clean, `DELETE FROM faults WHERE device_id=$1`, device)
		db.Exec(clean, `DELETE FROM devices WHERE id=$1`, device)
		for _, id := range actors {
			db.Exec(clean, `DELETE FROM users WHERE id=$1`, id)
		}
	}()
	app := (&httpapi.Server{DB: db, Secret: strings.Repeat("s", 32), IngestToken: strings.Repeat("i", 32)}).Routes()
	request := func(method, path, token string, body any) *httptest.ResponseRecorder {
		var content string
		if body != nil {
			b, _ := json.Marshal(body)
			content = string(b)
		}
		r := httptest.NewRequest(method, path, strings.NewReader(content))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		return w
	}
	tokens := map[string]string{}
	for key := range actors {
		w := request("POST", "/api/auth/login", "", map[string]string{"email": key + nonce + "@local", "password": "test-password-123"})
		var session struct {
			Token string `json:"token"`
		}
		if err = json.Unmarshal(w.Body.Bytes(), &session); err != nil || w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		tokens[key] = session.Token
	}
	report := domain.ManualReport{RequestID: "initial-report", DeviceID: device, FaultCode: "CONNECTION_TIMEOUT", Severity: "medium", Description: "Sahada bağlantı aralıklı olarak kesiliyor", LocationDetail: "Pano 12, kuzey yönü", FieldNote: "Bağlantı LED'i aralıklı sönüyor"}
	if w := request("POST", "/api/fault-reports", "", report); w.Code != 401 {
		t.Fatal("anonymous report accepted")
	}
	forged := map[string]any{"request_id": "forged-key", "device_id": device, "fault_code": "CONNECTION_TIMEOUT", "severity": "high", "description": "Yeterince uzun açıklama", "reporter_id": actors["admin"]}
	if w := request("POST", "/api/fault-reports", tokens["tech"], forged); w.Code != 400 {
		t.Fatal("forged reporter accepted")
	}
	bad := report
	bad.DeviceID = "unknown-device"
	if w := request("POST", "/api/fault-reports", tokens["tech"], bad); w.Code != 422 {
		t.Fatal("unknown device accepted")
	}
	created := request("POST", "/api/fault-reports", tokens["tech"], report)
	var receipt platform.Receipt
	if err = json.Unmarshal(created.Body.Bytes(), &receipt); created.Code != 201 || err != nil || !receipt.CreatedFault || receipt.Duplicate {
		t.Fatal("new report failed", created.Body.String(), err)
	}
	var status, source, location string
	var assignee *int64
	var reporterID int64
	if err = db.QueryRow(ctx, `SELECT w.status,w.assignee_id,e.source,e.reporter_id,e.payload->>'location' FROM events e JOIN work_orders w ON w.fault_id=e.fault_id WHERE e.event_id=$1`, receipt.EventID).Scan(&status, &assignee, &source, &reporterID, &location); err != nil || status != "open" || assignee != nil || source != "manual" || reporterID != actors["tech"] || location != "Kadıköy" {
		t.Fatal("invalid provenance/assignment", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := request("POST", "/api/fault-reports", tokens["tech"], report)
			var result platform.Receipt
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || !result.Duplicate || result.FaultID != receipt.FaultID {
				t.Error("retry created additional work", w.Body.String())
			}
		}()
	}
	wg.Wait()
	changed := report
	changed.Description = "Aynı anahtarla değiştirilen başka açıklama"
	if w := request("POST", "/api/fault-reports", tokens["tech"], changed); w.Code != 409 {
		t.Fatal("changed retry accepted")
	}
	path := fmt.Sprintf("/api/work-orders/%d", receipt.WorkOrderID)
	if w := request("PATCH", path, tokens["admin"], map[string]any{"status": "assigned", "assignee_id": actors["other"]}); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	report.RequestID = "second-report"
	report.Severity = "high"
	report.Description = "Aynı sorun tekrar gözlendi, cihaz cevap vermiyor"
	merged := request("POST", "/api/fault-reports", tokens["tech"], report)
	var second platform.Receipt
	if err = json.Unmarshal(merged.Body.Bytes(), &second); merged.Code != 201 || err != nil || second.CreatedFault || second.FaultID != receipt.FaultID || second.WorkOrderID != receipt.WorkOrderID {
		t.Fatal("merge failed", merged.Body.String())
	}
	// A different actor may use the same request key without impersonating the first.
	report.Description = "Başka teknisyenin aynı arızaya ilişkin gözlemi"
	if w := request("POST", "/api/fault-reports", tokens["other"], report); w.Code != 201 {
		t.Fatal("actor request namespace collision", w.Body.String())
	}
	// A simulator event uses the same aggregation path without claiming a person.
	event := domain.Event{Source: "simulator", EventID: "sim-" + nonce, DeviceID: device, FaultCode: "CONNECTION_TIMEOUT", Severity: "low", Location: "Kadıköy", Description: "Automatic simulator observation", Timestamp: time.Now()}
	if err = platform.ApplyEvent(ctx, db, event); err != nil {
		t.Fatal(err)
	}
	var count int
	var severity string
	if err = db.QueryRow(ctx, `SELECT occurrences,severity FROM faults WHERE id=$1`, receipt.FaultID).Scan(&count, &severity); err != nil || count != 4 || severity != "high" {
		t.Fatal("merged count/severity incorrect", count, severity, err)
	}
	if err = db.QueryRow(ctx, `SELECT status,assignee_id FROM work_orders WHERE id=$1`, receipt.WorkOrderID).Scan(&status, &assignee); err != nil || status != "assigned" || assignee == nil || *assignee != actors["other"] {
		t.Fatal("merge changed assignment", err)
	}
	for key, want := range map[string]int{"admin": 3, "tech": 2, "other": 1} {
		w := request("GET", "/api/fault-reports", tokens[key], nil)
		var rows []map[string]any
		if err = json.Unmarshal(w.Body.Bytes(), &rows); err != nil || w.Code != 200 || len(rows) != want {
			t.Fatal("manual report visibility", key, w.Body.String())
		}
	}
	eventsPath := fmt.Sprintf("/api/faults/%d/events", receipt.FaultID)
	if w := request("GET", eventsPath, tokens["tech"], nil); w.Code != 404 {
		t.Fatal("reporting conferred access to another assignee's work")
	}
	if w := request("GET", eventsPath, tokens["other"], nil); w.Code != 200 || !strings.Contains(w.Body.String(), "simulator") || !strings.Contains(w.Body.String(), "Pano 12") {
		t.Fatal("event provenance missing", w.Body.String())
	}
	for _, key := range []string{"admin", "operator"} {
		r := report
		r.RequestID = "new-" + key
		r.FaultCode = "CUSTOM_" + strings.ToUpper(key)
		if w := request("POST", "/api/fault-reports", tokens[key], r); w.Code != 201 {
			t.Fatal("management role could not report", w.Body.String())
		}
	}
	var audits int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE work_order_id=$1 AND action='manual_report'`, receipt.WorkOrderID).Scan(&audits); err != nil || audits != 3 {
		t.Fatal("manual audit missing or duplicated", audits, err)
	}
}
