package httpapi

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"time"
	"urbanops/internal/assistant"
)

func (s *Server) similarSolutions(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
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
	records, err := assistant.HistoricalSolutions(r.Context(), s.DB, id)
	if err != nil {
		s.failure(w, err)
		return
	}
	jsonResponse(w, 200, records)
}
func (s *Server) documentDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		problem(w, 400, "Geçersiz kimlik")
		return
	}
	var title, content string
	if err = s.DB.QueryRow(r.Context(), `SELECT title,content FROM documents WHERE id=$1`, id).Scan(&title, &content); errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "Belge bulunamadı")
		return
	} else if err != nil {
		s.failure(w, err)
		return
	}
	jsonResponse(w, 200, map[string]any{"id": id, "title": title, "content": content})
}
func (s *Server) retryDocumentIndex(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		problem(w, 400, "Geçersiz kimlik")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	err = s.AI.IndexExisting(ctx, s.DB, id)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "Belge bulunamadı")
		return
	}
	if err != nil {
		problem(w, 503, "İndeks oluşturulamadı; belge ve önceki indeks korundu. Ollama servisini kontrol edin.")
		return
	}
	jsonResponse(w, 200, map[string]any{"id": id, "status": "indexed"})
}
