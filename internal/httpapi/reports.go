package httpapi

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"time"
	"urbanops/internal/domain"
	"urbanops/internal/platform"
)

func (s *Server) createFaultReport(w http.ResponseWriter, r *http.Request) {
	var in domain.ManualReport
	if !decode(w, r, &in) {
		return
	}
	in.Normalize()
	if err := in.Validate(); err != nil {
		problem(w, 422, err.Error())
		return
	}
	var location string
	if err := s.DB.QueryRow(r.Context(), `SELECT location FROM devices WHERE id=$1`, in.DeviceID).Scan(&location); errors.Is(err, pgx.ErrNoRows) {
		problem(w, 422, "Cihaz kayıtlı değil; önce yöneticinizden cihazı envantere eklemesini isteyin")
		return
	} else if err != nil {
		s.failure(w, err)
		return
	}
	c := r.Context().Value(authKey{}).(Claims)
	observed := in.ObservedAt
	if observed.IsZero() {
		observed = time.Now()
	}
	e := domain.Event{EventID: fmt.Sprintf("manual-%d-%s", c.ID, in.RequestID), DeviceID: in.DeviceID, FaultCode: in.FaultCode, Severity: in.Severity, Location: location, Description: in.Description, Timestamp: observed}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	receipt, err := platform.ApplyManualReport(ctx, s.DB, e, c.ID, in)
	if errors.Is(err, platform.ErrReportConflict) {
		problem(w, 409, "Bu bildirim anahtarı farklı bilgilerle kullanılmış; yeni bildirim oluşturun")
		return
	}
	if err != nil {
		s.failure(w, err)
		return
	}
	status := 201
	if receipt.Duplicate {
		status = 200
	}
	jsonResponse(w, status, receipt)
}
func (s *Server) faultReports(w http.ResponseWriter, r *http.Request) {
	s.list(w, r, `SELECT e.event_id,e.fault_id,w.id AS work_order_id,e.received_at,e.payload->>'timestamp' AS observed_at,e.payload->>'description' AS description,e.payload->>'severity' AS severity,
 e.location_detail,e.field_note,e.source,u.name AS reporter,d.id AS device_id,d.name AS device_name,d.location,f.fault_code,w.status AS work_order_status
 FROM events e JOIN faults f ON f.id=e.fault_id JOIN devices d ON d.id=f.device_id JOIN work_orders w ON w.fault_id=f.id JOIN users u ON u.id=e.reporter_id
 WHERE e.source='manual' AND ($1::bigint=0 OR e.reporter_id=$1) ORDER BY e.received_at DESC,e.event_id LIMIT 500`, technicianScope(r))
}
func (s *Server) faultEvents(w http.ResponseWriter, r *http.Request) {
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
	s.list(w, r, `SELECT e.event_id,e.received_at,e.payload->>'timestamp' AS observed_at,e.payload->>'description' AS description,e.payload->>'severity' AS severity,e.source,e.location_detail,e.field_note,u.name AS reporter
 FROM events e LEFT JOIN users u ON u.id=e.reporter_id WHERE e.fault_id=$1 ORDER BY e.received_at DESC,e.event_id LIMIT 100`, id)
}
