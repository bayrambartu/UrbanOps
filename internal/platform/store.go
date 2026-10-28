package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"urbanops/internal/domain"
)

var ErrReportConflict = errors.New("bildirim anahtarı farklı bilgilerle zaten kullanılmış")

type Receipt struct {
	EventID      string `json:"event_id"`
	FaultID      int64  `json:"fault_id"`
	WorkOrderID  int64  `json:"work_order_id"`
	CreatedFault bool   `json:"created_fault"`
	Duplicate    bool   `json:"duplicate"`
}
type EventOrigin struct {
	Source                                 string
	ReporterID                             *int64
	LocationDetail, FieldNote, Fingerprint string
}

// Both human reports and worker events share one aggregation transaction.
func ApplyEvent(ctx context.Context, db *pgxpool.Pool, e domain.Event) error {
	source := e.Source
	if source == "" {
		source = "device"
	}
	_, err := applyEvent(ctx, db, e, EventOrigin{Source: source})
	return err
}
func ApplyManualReport(ctx context.Context, db *pgxpool.Pool, e domain.Event, reporterID int64, report domain.ManualReport) (Receipt, error) {
	return applyEvent(ctx, db, e, EventOrigin{Source: "manual", ReporterID: &reporterID, LocationDetail: report.LocationDetail, FieldNote: report.FieldNote, Fingerprint: report.Fingerprint()})
}
func applyEvent(ctx context.Context, db *pgxpool.Pool, e domain.Event, origin EventOrigin) (Receipt, error) {
	result := Receipt{EventID: e.EventID}
	if err := e.Validate(); err != nil {
		return result, err
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, e.EventID); err != nil {
		return result, err
	}
	var existingFingerprint string
	err = tx.QueryRow(ctx, `SELECT e.fault_id,w.id,e.request_fingerprint FROM events e JOIN work_orders w ON w.fault_id=e.fault_id WHERE e.event_id=$1`, e.EventID).Scan(&result.FaultID, &result.WorkOrderID, &existingFingerprint)
	if err == nil {
		if origin.Source == "manual" && existingFingerprint != origin.Fingerprint {
			return result, ErrReportConflict
		}
		result.Duplicate = true
		return result, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,1))`, e.DeviceID+":"+e.FaultCode); err != nil {
		return result, err
	}
	var location string
	if err = tx.QueryRow(ctx, `SELECT location FROM devices WHERE id=$1`, e.DeviceID).Scan(&location); err != nil {
		return result, fmt.Errorf("cihaz bulunamadı: %w", err)
	}
	if location != e.Location {
		return result, fmt.Errorf("cihaz konumu eşleşmiyor")
	}
	var active bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM faults WHERE device_id=$1 AND fault_code=$2 AND status='open')`, e.DeviceID, e.FaultCode).Scan(&active); err != nil {
		return result, err
	}
	result.CreatedFault = !active
	err = tx.QueryRow(ctx, `INSERT INTO faults(device_id,fault_code,description,severity,first_seen,last_seen)
 VALUES($1,$2,$3,$4,$5,$5)
 ON CONFLICT(device_id,fault_code) WHERE status='open' DO UPDATE SET
 occurrences=faults.occurrences+1,first_seen=LEAST(faults.first_seen,EXCLUDED.first_seen),last_seen=GREATEST(faults.last_seen,EXCLUDED.last_seen),
 severity=CASE WHEN array_position(ARRAY['low','medium','high','critical'],EXCLUDED.severity)>array_position(ARRAY['low','medium','high','critical'],faults.severity) THEN EXCLUDED.severity ELSE faults.severity END
 RETURNING id`, e.DeviceID, e.FaultCode, e.Description, e.Severity, e.Timestamp).Scan(&result.FaultID)
	if err != nil {
		return result, err
	}
	payload, _ := json.Marshal(e)
	if _, err = tx.Exec(ctx, `INSERT INTO events(event_id,fault_id,payload,source,reporter_id,location_detail,field_note,request_fingerprint) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, e.EventID, result.FaultID, payload, origin.Source, origin.ReporterID, origin.LocationDetail, origin.FieldNote, origin.Fingerprint); err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO work_orders(fault_id) VALUES($1) ON CONFLICT DO NOTHING`, result.FaultID); err != nil {
		return result, err
	}
	if err = tx.QueryRow(ctx, `SELECT id FROM work_orders WHERE fault_id=$1`, result.FaultID).Scan(&result.WorkOrderID); err != nil {
		return result, err
	}
	if origin.ReporterID != nil {
		note := e.Description
		if origin.LocationDetail != "" {
			note += "\nKonum detayı: " + origin.LocationDetail
		}
		if origin.FieldNote != "" {
			note += "\nSaha notu: " + origin.FieldNote
		}
		if _, err = tx.Exec(ctx, `INSERT INTO audit_log(work_order_id,actor_id,action,note) VALUES($1,$2,'manual_report',$3)`, result.WorkOrderID, *origin.ReporterID, note); err != nil {
			return result, err
		}
	}
	return result, tx.Commit(ctx)
}
func RowsJSON(ctx context.Context, db *pgxpool.Pool, query string, args ...any) ([]map[string]any, error) {
	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]map[string]any, 0)
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		item := map[string]any{}
		for i, f := range rows.FieldDescriptions() {
			item[f.Name] = values[i]
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
