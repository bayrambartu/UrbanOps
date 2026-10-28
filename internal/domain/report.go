package domain

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var reportKey = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)
var faultCode = regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,63}$`)

type ManualReport struct {
	RequestID      string    `json:"request_id"`
	DeviceID       string    `json:"device_id"`
	FaultCode      string    `json:"fault_code"`
	Severity       string    `json:"severity"`
	Description    string    `json:"description"`
	ObservedAt     time.Time `json:"observed_at,omitempty"`
	LocationDetail string    `json:"location_detail"`
	FieldNote      string    `json:"field_note"`
}

func (r *ManualReport) Normalize() {
	r.DeviceID = strings.TrimSpace(r.DeviceID)
	r.FaultCode = strings.ToUpper(strings.TrimSpace(r.FaultCode))
	r.Description = strings.TrimSpace(r.Description)
	r.LocationDetail = strings.TrimSpace(r.LocationDetail)
	r.FieldNote = strings.TrimSpace(r.FieldNote)
}
func (r ManualReport) Validate() error {
	if !reportKey.MatchString(r.RequestID) {
		return fmt.Errorf("geçersiz bildirim anahtarı")
	}
	if r.DeviceID == "" || len(r.DeviceID) > 200 {
		return fmt.Errorf("kayıtlı bir cihaz seçin")
	}
	if !faultCode.MatchString(r.FaultCode) {
		return fmt.Errorf("hata kodu 3–64 karakter olmalı; büyük harf, rakam ve alt çizgi kullanılabilir")
	}
	if utf8.RuneCountInString(r.Description) < 10 || len(r.Description) > 2000 {
		return fmt.Errorf("arıza açıklaması en az 10 karakter olmalı ve 2000 baytı aşmamalı")
	}
	switch r.Severity {
	case "low", "medium", "high", "critical":
	default:
		return fmt.Errorf("önem derecesi seçin")
	}
	if !r.ObservedAt.IsZero() && r.ObservedAt.After(time.Now().Add(5*time.Minute)) {
		return fmt.Errorf("gözlem zamanı gelecekte olamaz")
	}
	if utf8.RuneCountInString(r.LocationDetail) > 300 {
		return fmt.Errorf("detaylı konum en fazla 300 karakter olabilir")
	}
	if utf8.RuneCountInString(r.FieldNote) > 1000 {
		return fmt.Errorf("saha notu en fazla 1000 karakter olabilir")
	}
	return nil
}

// Hash the normalized request, not server defaults such as the current time.
// Retrying a request with no observed_at must still be idempotent.
func (r ManualReport) Fingerprint() string {
	r.RequestID = ""
	b, _ := json.Marshal(r)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
