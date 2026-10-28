package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"urbanops/internal/assistant"
	"urbanops/internal/domain"
	"urbanops/internal/platform"
)

type Server struct {
	DB                  *pgxpool.Pool
	Broker              *platform.Broker
	Secret, IngestToken string
	AI                  *assistant.Client
}

func jsonResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}
func problem(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]string{"error": message})
}
func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		problem(w, 400, "Geçersiz JSON veya istek çok büyük")
		return false
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		problem(w, 400, "Tek bir JSON nesnesi gönderin")
		return false
	}
	return true
}
func (s *Server) failure(w http.ResponseWriter, err error) {
	slog.Error("API işlemi başarısız", "error", err)
	problem(w, 500, "İşlem tamamlanamadı")
}
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { jsonResponse(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.DB.Ping(ctx); err != nil || s.Broker.Conn.IsClosed() || s.Broker.Ch.IsClosed() {
			problem(w, 503, "Bağımlılıklar hazır değil")
			return
		}
		jsonResponse(w, 200, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("POST /api/auth/login", limitLogin(s.login))
	mux.HandleFunc("GET /api/me", s.protected(func(w http.ResponseWriter, r *http.Request) {
		c := r.Context().Value(authKey{}).(Claims)
		jsonResponse(w, 200, c)
	}))
	mux.HandleFunc("POST /api/events", s.ingest)
	mux.HandleFunc("POST /api/fault-reports", s.protected(s.createFaultReport))
	mux.HandleFunc("GET /api/fault-reports", s.protected(s.faultReports))
	mux.HandleFunc("GET /api/faults/{id}/events", s.protected(s.faultEvents))
	mux.HandleFunc("GET /api/devices", s.protected(s.devices))
	mux.HandleFunc("POST /api/devices", s.protected(s.createDevice, "admin", "operator"))
	mux.HandleFunc("GET /api/faults", s.protected(s.faults))
	mux.HandleFunc("GET /api/work-orders", s.protected(s.orders))
	mux.HandleFunc("PATCH /api/work-orders/{id}", s.protected(s.updateOrder))
	mux.HandleFunc("GET /api/work-orders/{id}/history", s.protected(s.history))
	mux.HandleFunc("GET /api/users", s.protected(s.users))
	mux.HandleFunc("GET /api/technicians", s.protected(s.technicians, "admin", "operator"))
	mux.HandleFunc("POST /api/users", s.protected(s.createUser, "admin"))
	mux.HandleFunc("GET /api/reports", s.protected(s.reports, "admin", "operator"))
	mux.HandleFunc("GET /api/documents", s.protected(s.documents))
	mux.HandleFunc("POST /api/documents", s.protected(s.indexDocument, "admin", "operator"))
	mux.HandleFunc("POST /api/faults/{id}/suggestions", s.protected(s.suggest))
	mux.HandleFunc("GET /api/faults/{id}/similar", s.protected(s.similarSolutions))
	mux.HandleFunc("GET /api/documents/{id}", s.protected(s.documentDetail))
	mux.HandleFunc("POST /api/documents/{id}/index", s.protected(s.retryDocumentIndex, "admin", "operator"))
	mux.Handle("GET /openapi.yaml", http.FileServer(http.Dir("docs")))
	mux.Handle("/", http.FileServer(http.Dir(platform.Env("WEB_DIR", "web/dist"))))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		mux.ServeHTTP(w, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(start).Milliseconds())
	})
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	var id int64
	var role, hash string
	err := s.DB.QueryRow(r.Context(), `SELECT id,role,password_hash FROM users WHERE email=$1`, strings.ToLower(strings.TrimSpace(in.Email))).Scan(&id, &role, &hash)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		problem(w, 401, "E-posta veya parola hatalı")
		return
	}
	jsonResponse(w, 200, map[string]any{"token": sign(Claims{id, role, time.Now().Add(8 * time.Hour).Unix()}, s.Secret), "role": role})
}
func (s *Server) ingest(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(token), []byte(s.IngestToken)) != 1 {
		problem(w, 401, "Geçersiz cihaz anahtarı")
		return
	}
	var e domain.Event
	if !decode(w, r, &e) {
		return
	}
	if err := e.Validate(); err != nil {
		problem(w, 422, err.Error())
		return
	}
	var location string
	err := s.DB.QueryRow(r.Context(), `SELECT location FROM devices WHERE id=$1`, e.DeviceID).Scan(&location)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 422, "Cihaz kayıtlı değil")
		return
	}
	if err != nil {
		s.failure(w, err)
		return
	}
	if location != e.Location {
		problem(w, 422, "Cihaz konumu eşleşmiyor")
		return
	}
	body, _ := json.Marshal(e)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err = s.Broker.Publish(ctx, platform.Queue, e.EventID, body, nil); err != nil {
		slog.Error("olay yayınlanamadı", "error", err)
		problem(w, 503, "Kuyruk onayı alınamadı; aynı event_id ile yeniden deneyin")
		return
	}
	jsonResponse(w, 202, map[string]string{"event_id": e.EventID, "status": "accepted"})
}
func (s *Server) list(w http.ResponseWriter, r *http.Request, query string, args ...any) {
	items, err := platform.RowsJSON(r.Context(), s.DB, query, args...)
	if err != nil {
		s.failure(w, err)
		return
	}
	jsonResponse(w, 200, items)
}
func (s *Server) devices(w http.ResponseWriter, r *http.Request) {
	s.list(w, r, `SELECT * FROM devices ORDER BY created_at DESC LIMIT 500`)
}
func (s *Server) createDevice(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Kind     string `json:"kind"`
		Location string `json:"location"`
	}
	if !decode(w, r, &in) {
		return
	}
	for _, v := range []string{in.ID, in.Name, in.Kind, in.Location} {
		if strings.TrimSpace(v) == "" || len(v) > 200 {
			problem(w, 422, "Cihaz alanları zorunlu ve en fazla 200 bayt olmalı")
			return
		}
	}
	tag, err := s.DB.Exec(r.Context(), `INSERT INTO devices(id,name,kind,location) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, in.ID, in.Name, in.Kind, in.Location)
	if err != nil {
		s.failure(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		problem(w, 409, "Cihaz kimliği zaten kayıtlı")
		return
	}
	jsonResponse(w, 201, in)
}
func (s *Server) faults(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	s.list(w, r, `SELECT f.*,d.name AS device_name,d.location,w.id AS work_order_id FROM faults f JOIN devices d ON d.id=f.device_id JOIN work_orders w ON w.fault_id=f.id WHERE ($1='' OR f.status=$1) AND ($2::bigint=0 OR w.assignee_id=$2) ORDER BY f.last_seen DESC LIMIT 500`, status, technicianScope(r))
}
func (s *Server) orders(w http.ResponseWriter, r *http.Request) {
	s.list(w, r, `SELECT w.*,f.device_id,f.fault_code,f.severity,f.resolution,f.description,f.occurrences,d.location,u.name AS assignee FROM work_orders w JOIN faults f ON f.id=w.fault_id JOIN devices d ON d.id=f.device_id LEFT JOIN users u ON u.id=w.assignee_id WHERE ($1::bigint=0 OR w.assignee_id=$1) ORDER BY CASE f.severity WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 ELSE 3 END,w.created_at,w.id LIMIT 500`, technicianScope(r))
}
func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		problem(w, 400, "Geçersiz kimlik")
		return
	}
	if !s.canReadOrder(w, r, id) {
		return
	}
	s.list(w, r, `SELECT a.*,u.name AS actor,p.name AS previous_assignee,n.name AS next_assignee FROM audit_log a JOIN users u ON u.id=a.actor_id LEFT JOIN users p ON p.id=a.previous_assignee_id LEFT JOIN users n ON n.id=a.next_assignee_id WHERE work_order_id=$1 ORDER BY a.id`, id)
}
func (s *Server) users(w http.ResponseWriter, r *http.Request) {
	s.list(w, r, `SELECT id,name,email,role FROM users WHERE ($1::bigint=0 OR id=$1) ORDER BY name`, technicianScope(r))
}
func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if !strings.Contains(in.Email, "@") || len(in.Email) > 254 || strings.TrimSpace(in.Name) == "" || len(in.Name) > 200 || len(in.Password) < 12 || len(in.Password) > 72 || (in.Role != "admin" && in.Role != "operator" && in.Role != "technician") {
		problem(w, 422, "Geçerli e-posta, ad, rol ve 12–72 bayt parola gerekli")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		s.failure(w, err)
		return
	}
	var id int64
	err = s.DB.QueryRow(r.Context(), `INSERT INTO users(email,name,password_hash,role) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING RETURNING id`, in.Email, in.Name, string(hash), in.Role).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 409, "E-posta zaten kayıtlı")
		return
	}
	if err != nil {
		s.failure(w, err)
		return
	}
	jsonResponse(w, 201, map[string]any{"id": id})
}
func (s *Server) reports(w http.ResponseWriter, r *http.Request) {
	result := map[string]any{}
	queries := map[string]string{
		"summary":     `SELECT count(*) AS total_faults,count(*) FILTER(WHERE status='open') AS open_faults,coalesce(avg(extract(epoch FROM (resolved_at-first_seen))/3600) FILTER(WHERE resolved_at>=first_seen),0) AS mean_resolution_hours,coalesce(sum(occurrences-1),0) AS repeated_events FROM faults`,
		"by_device":   `SELECT device_id,count(*) AS faults,sum(occurrences) AS events FROM faults GROUP BY device_id ORDER BY events DESC LIMIT 10`,
		"by_code":     `SELECT fault_code,count(*) AS faults FROM faults GROUP BY fault_code ORDER BY faults DESC`,
		"by_location": `SELECT d.location,count(*) AS faults FROM faults f JOIN devices d ON d.id=f.device_id GROUP BY d.location ORDER BY faults DESC`,
		"work_orders": `SELECT status,count(*) AS count FROM work_orders GROUP BY status`,
	}
	for key, q := range queries {
		rows, err := platform.RowsJSON(r.Context(), s.DB, q)
		if err != nil {
			s.failure(w, err)
			return
		}
		result[key] = rows
	}
	jsonResponse(w, 200, result)
}
func (s *Server) documents(w http.ResponseWriter, r *http.Request) {
	s.list(w, r, `SELECT d.id,d.title,d.created_at,count(c.id) AS chunks,CASE WHEN count(c.id)>0 THEN 'indexed' ELSE 'pending' END AS status FROM documents d LEFT JOIN chunks c ON c.document_id=d.id GROUP BY d.id ORDER BY d.id DESC`)
}
func (s *Server) indexDocument(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Title) == "" || len(in.Title) > 300 || len([]rune(strings.TrimSpace(in.Content))) < 30 || len(in.Content) > 100000 {
		problem(w, 422, "Başlık ve 30 karakterden uzun metin gerekli; en fazla 100 KB")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	id, err := assistant.SaveDocument(r.Context(), s.DB, in.Title, in.Content)
	if err != nil {
		s.failure(w, err)
		return
	}
	err = s.AI.IndexExisting(ctx, s.DB, id)
	if err != nil {
		slog.Error("indeksleme başarısız", "error", err)
		jsonResponse(w, 202, map[string]any{"id": id, "status": "pending", "message": "Belge kaydedildi. Vektör indeksi oluşturulamadı; Ollama hazır olduğunda yeniden indeksleyin."})
		return
	}
	jsonResponse(w, 201, map[string]any{"id": id, "status": "indexed"})
}
func (s *Server) suggest(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		problem(w, 400, "Geçersiz kimlik")
		return
	}
	var orderID int64
	if err = s.DB.QueryRow(r.Context(), `SELECT id FROM work_orders WHERE fault_id=$1`, id).Scan(&orderID); errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "Arıza bulunamadı")
		return
	} else if err != nil {
		s.failure(w, err)
		return
	}
	if !s.canReadOrder(w, r, orderID) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	result, err := s.AI.Suggest(ctx, s.DB, id)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "Arıza bulunamadı")
		return
	}
	if err != nil {
		slog.Error("öneri üretilemedi", "error", err)
		problem(w, 503, "AI servisi kullanılamıyor; Ollama ve modelleri kontrol edin")
		return
	}
	jsonResponse(w, 200, result)
}
