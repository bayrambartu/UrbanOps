package domain

import (
	"strings"
	"testing"
	"time"
)

func validReport() ManualReport {
	return ManualReport{RequestID: "12345678-report", DeviceID: "TRF-104", FaultCode: "CONNECTION_TIMEOUT", Severity: "high", Description: "Bağlantı aralıklı olarak kesiliyor"}
}
func TestManualReportValidation(t *testing.T) {
	base := validReport()
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		edit func(*ManualReport)
	}{
		{"missing device", func(r *ManualReport) { r.DeviceID = "" }},
		{"short description", func(r *ManualReport) { r.Description = "Hata" }},
		{"unicode description too short", func(r *ManualReport) { r.Description = "İşçığüşö" }},
		{"oversized description", func(r *ManualReport) { r.Description = strings.Repeat("a", 2001) }},
		{"invalid key", func(r *ManualReport) { r.RequestID = "../key" }},
		{"invalid code", func(r *ManualReport) { r.FaultCode = "BAD CODE" }},
		{"oversized code", func(r *ManualReport) { r.FaultCode = strings.Repeat("A", 65) }},
		{"invalid severity", func(r *ManualReport) { r.Severity = "urgent" }},
		{"future observation", func(r *ManualReport) { r.ObservedAt = time.Now().Add(time.Hour) }},
		{"oversized location", func(r *ManualReport) { r.LocationDetail = strings.Repeat("İ", 301) }},
		{"oversized note", func(r *ManualReport) { r.FieldNote = strings.Repeat("a", 1001) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := base
			tt.edit(&r)
			if r.Validate() == nil {
				t.Fatal("invalid manual report accepted")
			}
		})
	}
}
func TestManualReportNormalizationAndRetryFingerprint(t *testing.T) {
	r := validReport()
	r.DeviceID = " TRF-104 "
	r.FaultCode = " connection_timeout "
	r.Description = "  Bağlantı aralıklı olarak kesiliyor  "
	r.Normalize()
	original := validReport()
	if r.Fingerprint() != original.Fingerprint() {
		t.Fatal("normalized request changed identity")
	}
	copy := r
	copy.RequestID = "another-key"
	if copy.Fingerprint() != r.Fingerprint() {
		t.Fatal("request key included in payload fingerprint")
	}
	copy.FieldNote = "Yeni gözlem"
	if copy.Fingerprint() == r.Fingerprint() {
		t.Fatal("changed content not detected")
	}
	if !r.ObservedAt.IsZero() {
		t.Fatal("server current-time default changed original request")
	}
}
func TestAutomaticEventCannotClaimManualSource(t *testing.T) {
	e := Event{Source: "manual", EventID: "evt", DeviceID: "D", FaultCode: "FAIL", Severity: "high", Location: "L", Description: "D", Timestamp: time.Now()}
	if e.Validate() == nil {
		t.Fatal("automatic event forged manual provenance")
	}
	e.Source = "simulator"
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
}
