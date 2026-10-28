package domain

import (
	"errors"
	"strings"
	"time"
)

type Event struct {
	Source      string    `json:"source,omitempty"`
	EventID     string    `json:"event_id"`
	DeviceID    string    `json:"device_id"`
	FaultCode   string    `json:"fault_code"`
	Severity    string    `json:"severity"`
	Location    string    `json:"location"`
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
}

func (e Event) Validate() error {
	if e.Source != "" && e.Source != "device" && e.Source != "simulator" {
		return errors.New("geçersiz otomatik bildirim kaynağı")
	}
	for _, s := range []string{e.EventID, e.DeviceID, e.FaultCode, e.Location, e.Description} {
		if strings.TrimSpace(s) == "" || len(s) > 2000 {
			return errors.New("alanlar boş olamaz ve 2000 baytı aşamaz")
		}
	}
	switch e.Severity {
	case "low", "medium", "high", "critical":
	default:
		return errors.New("geçersiz önem derecesi")
	}
	if e.Timestamp.IsZero() || e.Timestamp.After(time.Now().Add(5*time.Minute)) {
		return errors.New("geçersiz olay zamanı")
	}
	return nil
}
func CanTransition(from, to string) bool {
	switch from {
	case "open":
		return to == "assigned"
	case "assigned":
		return to == "in_progress"
	case "in_progress":
		return to == "resolved" || to == "on_hold"
	case "on_hold":
		return to == "in_progress"
	case "resolved":
		return to == "closed"
	}
	return false
}
