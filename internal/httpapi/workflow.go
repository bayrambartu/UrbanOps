package httpapi

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"strings"
	"urbanops/internal/domain"
)

func technicianScope(r *http.Request) int64 {
	c := r.Context().Value(authKey{}).(Claims)
	if c.Role == "technician" {
		return c.ID
	}
	return 0
}
func (s *Server) canReadOrder(w http.ResponseWriter, r *http.Request, id int64) bool {
	var visible bool
	err := s.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM work_orders WHERE id=$1 AND ($2::bigint=0 OR assignee_id=$2))`, id, technicianScope(r)).Scan(&visible)
	if err != nil {
		s.failure(w, err)
		return false
	}
	if !visible {
		problem(w, 404, "İş emri bulunamadı")
		return false
	}
	return true
}
func (s *Server) technicians(w http.ResponseWriter, r *http.Request) {
	s.list(w, r, `SELECT u.id,u.name,u.email,
 count(w.id) FILTER(WHERE w.status='assigned') AS assigned_count,
 count(w.id) FILTER(WHERE w.status='in_progress') AS in_progress_count,
 count(w.id) FILTER(WHERE w.status='on_hold') AS on_hold_count,
 count(w.id) FILTER(WHERE w.reassignment_requested AND w.status IN ('assigned','in_progress','on_hold')) AS transfer_requests
 FROM users u LEFT JOIN work_orders w ON w.assignee_id=u.id WHERE u.role='technician' GROUP BY u.id ORDER BY count(w.id) FILTER(WHERE w.status='in_progress'),count(w.id) FILTER(WHERE w.status IN ('assigned','on_hold')),u.name,u.id`)
}
func (s *Server) updateOrder(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		problem(w, 400, "Geçersiz kimlik")
		return
	}
	var in domain.OrderChange
	if !decode(w, r, &in) {
		return
	}
	in.Note = strings.TrimSpace(in.Note)
	c := r.Context().Value(authKey{}).(Claims)
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		s.failure(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	// Serialize with event aggregation before locking the work order. Reopening a
	// solution cannot race a newly arriving open fault for the same device/code.
	var device, code string
	err = tx.QueryRow(r.Context(), `SELECT f.device_id,f.fault_code FROM work_orders w JOIN faults f ON f.id=w.fault_id WHERE w.id=$1 AND ($2::bigint=0 OR w.assignee_id=$2)`, id, technicianScope(r)).Scan(&device, &code)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "İş emri bulunamadı")
		return
	}
	if err != nil {
		s.failure(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,1))`, device+":"+code); err != nil {
		s.failure(w, err)
		return
	}
	var current domain.OrderState
	var faultID int64
	err = tx.QueryRow(r.Context(), `SELECT status,assignee_id,fault_id,reassignment_requested FROM work_orders WHERE id=$1 FOR UPDATE`, id).Scan(&current.Status, &current.AssigneeID, &faultID, &current.ReassignmentRequested)
	if err != nil {
		s.failure(w, err)
		return
	}
	target, err := domain.PlanOrderChange(current, domain.Actor{ID: c.ID, Role: c.Role}, in)
	if err != nil {
		var policy *domain.WorkflowError
		if errors.As(err, &policy) {
			status := 422
			if policy.Code == "forbidden" {
				status = 403
			}
			if policy.Code == "conflict" {
				status = 409
			}
			problem(w, status, policy.Message)
			return
		}
		s.failure(w, err)
		return
	}
	assignee := current.AssigneeID
	if in.AssigneeID != nil {
		var role string
		err = tx.QueryRow(r.Context(), `SELECT role FROM users WHERE id=$1`, *in.AssigneeID).Scan(&role)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && role != "technician" {
			problem(w, 422, "Geçerli bir teknisyen seçin")
			return
		}
		if err != nil {
			s.failure(w, err)
			return
		}
		assignee = in.AssigneeID
	}
	if in.Action == "return" {
		var otherOpen bool
		if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM faults WHERE device_id=$1 AND fault_code=$2 AND status='open' AND id<>$3)`, device, code, faultID).Scan(&otherOpen); err != nil {
			s.failure(w, err)
			return
		}
		if otherOpen {
			problem(w, 409, "Aynı cihaz ve hata kodu için yeni bir açık arıza var; mevcut işi kapatıp yeni iş üzerinden devam edin")
			return
		}
	}
	holdReason := ""
	if target == "on_hold" {
		holdReason = in.Note
	}
	requested := false
	reason := ""
	if in.Action == "request_reassignment" {
		requested = true
		reason = in.Note
	} else if in.Action == "" && (target == "in_progress" || target == "on_hold") {
		requested = current.ReassignmentRequested
		if requested {
			if err = tx.QueryRow(r.Context(), `SELECT reassignment_reason FROM work_orders WHERE id=$1`, id).Scan(&reason); err != nil {
				s.failure(w, err)
				return
			}
		}
	}
	// A transfer request does not alter the existing hold reason.
	if in.Action == "request_reassignment" {
		if err = tx.QueryRow(r.Context(), `SELECT hold_reason FROM work_orders WHERE id=$1`, id).Scan(&holdReason); err != nil {
			s.failure(w, err)
			return
		}
	}
	if _, err = tx.Exec(r.Context(), `UPDATE work_orders SET status=$1,assignee_id=$2,hold_reason=$3,reassignment_requested=$4,reassignment_reason=$5,updated_at=now() WHERE id=$6`, target, assignee, holdReason, requested, reason, id); err != nil {
		s.failure(w, err)
		return
	}
	if target == "resolved" && current.Status != "resolved" {
		_, err = tx.Exec(r.Context(), `UPDATE faults SET status='resolved',resolution=$1,resolved_at=now() WHERE id=$2`, in.Note, faultID)
	} else if in.Action == "return" {
		_, err = tx.Exec(r.Context(), `UPDATE faults SET status='open',resolution='',resolved_at=NULL WHERE id=$1`, faultID)
	}
	if err != nil {
		s.failure(w, err)
		return
	}
	action := current.Status + " → " + target
	if in.Action != "" {
		action = in.Action
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_log(work_order_id,actor_id,action,note,previous_assignee_id,next_assignee_id) VALUES($1,$2,$3,$4,$5,$6)`, id, c.ID, action, in.Note, current.AssigneeID, assignee); err != nil {
		s.failure(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		s.failure(w, err)
		return
	}
	jsonResponse(w, 200, map[string]string{"status": target})
}
