package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
	"urbanops/internal/domain"
	"urbanops/internal/platform"
)

func main() {
	ctx, stop := platform.Context()
	defer stop()
	client := &http.Client{Timeout: 10 * time.Second}
	token := platform.Required("INGEST_TOKEN")
	url := platform.Env("API_URL", "http://localhost:8080")
	interval, err := time.ParseDuration(platform.Env("SIM_INTERVAL", "10s"))
	if err != nil || interval < time.Second {
		panic("SIM_INTERVAL en az 1s olmalı")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	// Keep a failed event's ID until acknowledged, including ambiguous timeouts.
	var pending *domain.Event
	for {
		if pending == nil {
			var id [16]byte
			if _, err = rand.Read(id[:]); err != nil {
				panic(err)
			}
			pending = &domain.Event{Source: "simulator", EventID: "sim-" + hex.EncodeToString(id[:]), DeviceID: platform.Env("SIM_DEVICE_ID", "TRF-104"), FaultCode: "CONNECTION_TIMEOUT", Severity: "high", Location: platform.Env("SIM_LOCATION", "Kadıköy"), Description: "Simülatör: cihaz bağlantısı zaman aşımına uğradı", Timestamp: time.Now()}
		}
		body, _ := json.Marshal(pending)
		req, err := http.NewRequestWithContext(ctx, "POST", url+"/api/events", bytes.NewReader(body))
		if err != nil {
			panic(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			slog.Error("olay gönderilemedi", "error", err)
		} else {
			resp.Body.Close()
			slog.Info("simülasyon olayı", "event_id", pending.EventID, "status", resp.StatusCode)
			if resp.StatusCode == 202 {
				pending = nil
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
