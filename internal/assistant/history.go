package assistant

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

// Only manager-approved (closed) solutions become shared maintenance knowledge.
type HistoricalSolution struct {
	FaultID      int64     `json:"fault_id"`
	WorkOrderID  int64     `json:"work_order_id"`
	DeviceID     string    `json:"device_id"`
	FaultCode    string    `json:"fault_code"`
	Resolution   string    `json:"resolution"`
	ResolvedAt   time.Time `json:"resolved_at"`
	Technician   string    `json:"technician"`
	ApprovedBy   string    `json:"approved_by"`
	ApprovalNote string    `json:"approval_note"`
	SameDevice   bool      `json:"same_device"`
}

func HistoricalSolutions(ctx context.Context, db *pgxpool.Pool, id int64) ([]HistoricalSolution, error) {
	var device, code string
	if err := db.QueryRow(ctx, `SELECT device_id,fault_code FROM faults WHERE id=$1`, id).Scan(&device, &code); err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `SELECT f.id,w.id,f.device_id,f.fault_code,f.resolution,f.resolved_at,
 coalesce(t.name,''),coalesce(a.name,''),coalesce(review.note,''),(f.device_id=$3)
 FROM faults f JOIN work_orders w ON w.fault_id=f.id
 LEFT JOIN users t ON t.id=w.assignee_id
 LEFT JOIN LATERAL (SELECT actor_id,note FROM audit_log WHERE work_order_id=w.id AND action='resolved → closed' ORDER BY id DESC LIMIT 1) review ON true
 LEFT JOIN users a ON a.id=review.actor_id
 WHERE f.status='resolved' AND w.status='closed' AND f.fault_code=$1 AND f.id<>$2 AND f.resolution<>''
 ORDER BY (f.device_id=$3) DESC,f.resolved_at DESC,f.id DESC LIMIT 5`, code, id, device)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []HistoricalSolution{}
	for rows.Next() {
		var h HistoricalSolution
		if err = rows.Scan(&h.FaultID, &h.WorkOrderID, &h.DeviceID, &h.FaultCode, &h.Resolution, &h.ResolvedAt, &h.Technician, &h.ApprovedBy, &h.ApprovalNote, &h.SameDevice); err != nil {
			return nil, err
		}
		result = append(result, h)
	}
	return result, rows.Err()
}
