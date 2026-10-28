package platform

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func Env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func Required(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error("gerekli ortam değişkeni eksik", "key", key)
		os.Exit(1)
	}
	return v
}
func Context() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}
