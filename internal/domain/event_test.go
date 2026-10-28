package domain

import (
	"testing"
	"time"
)

func TestEventValidation(t *testing.T) {
	base := Event{EventID: "evt-1", DeviceID: "TRF-104", FaultCode: "TIMEOUT", Severity: "high", Location: "Kadıköy", Description: "Bağlantı kaybı", Timestamp: time.Now()}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		edit func(*Event)
	}{{"missing id", func(e *Event) { e.EventID = " " }}, {"invalid severity", func(e *Event) { e.Severity = "urgent" }}, {"missing timestamp", func(e *Event) { e.Timestamp = time.Time{} }}, {"future timestamp", func(e *Event) { e.Timestamp = time.Now().Add(time.Hour) }}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := base
			tt.edit(&e)
			if e.Validate() == nil {
				t.Fatal("invalid event accepted")
			}
		})
	}
}
func TestWorkOrderStateMachine(t *testing.T) {
	states := []string{"open", "assigned", "in_progress", "on_hold", "resolved", "closed"}
	allowed := map[string]bool{"open:assigned": true, "assigned:in_progress": true, "in_progress:on_hold": true, "on_hold:in_progress": true, "in_progress:resolved": true, "resolved:closed": true}
	for _, from := range states {
		for _, to := range states {
			want := allowed[from+":"+to]
			if got := CanTransition(from, to); got != want {
				t.Errorf("%s -> %s: got %v want %v", from, to, got, want)
			}
		}
	}
	if CanTransition("unknown", "assigned") {
		t.Fatal("unknown state accepted")
	}
}
