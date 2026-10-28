package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"golang.org/x/crypto/bcrypt"
	"log/slog"
	"net/http"
	"os"
	"time"
	"urbanops/docs/training"
	"urbanops/internal/assistant"
	"urbanops/internal/httpapi"
	"urbanops/internal/platform"
	"urbanops/migrations"
)

func main() {
	if err := run(); err != nil {
		slog.Error("API durdu", "error", err)
		os.Exit(1)
	}
}
func run() error {
	ctx, stop := platform.Context()
	defer stop()
	secret := platform.Required("JWT_SECRET")
	ingest := platform.Required("INGEST_TOKEN")
	if len(secret) < 32 || len(ingest) < 24 {
		return fmt.Errorf("JWT_SECRET en az 32, INGEST_TOKEN en az 24 bayt olmalı")
	}
	db, err := pgxpool.New(ctx, platform.Required("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer db.Close()
	if err = migrations.Apply(ctx, db); err != nil {
		return err
	}
	if platform.Env("IMPORT_TRAINING_DOCS", "false") == "true" {
		if err = training.ImportDocuments(ctx, db); err != nil {
			return err
		}
		slog.Info("eğitim kaynakları metin olarak hazır; vektör indeksi ayrıca oluşturulabilir")
	}
	if email := os.Getenv("ADMIN_EMAIL"); email != "" {
		password := platform.Required("ADMIN_PASSWORD")
		if len(password) < 12 || len(password) > 72 {
			return fmt.Errorf("yönetici parolası 12–72 bayt olmalı")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if _, err = db.Exec(ctx, `INSERT INTO users(email,name,password_hash,role) VALUES(lower($1),'Sistem Yöneticisi',$2,'admin') ON CONFLICT DO NOTHING`, email, string(hash)); err != nil {
			return err
		}
	}
	b, err := platform.OpenBroker(platform.Required("RABBITMQ_URL"))
	if err != nil {
		return err
	}
	defer b.Close()
	app := &httpapi.Server{DB: db, Broker: b, Secret: secret, IngestToken: ingest, AI: assistant.New(platform.Env("OLLAMA_URL", "http://localhost:11434"), platform.Env("OLLAMA_EMBED_MODEL", "nomic-embed-text"), platform.Env("OLLAMA_CHAT_MODEL", "qwen2.5:7b"))}
	srv := &http.Server{Addr: platform.Env("ADDR", ":8080"), Handler: app.Routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 150 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	result := make(chan error, 1)
	go func() { slog.Info("UrbanOps API hazır", "addr", srv.Addr); result <- srv.ListenAndServe() }()
	select {
	case err := <-result:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	case err := <-b.Conn.NotifyClose(make(chan *amqp.Error, 1)):
		stop()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
		return fmt.Errorf("RabbitMQ bağlantısı kapandı: %v", err)
	case err := <-b.Ch.NotifyClose(make(chan *amqp.Error, 1)):
		stop()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
		return fmt.Errorf("RabbitMQ kanalı kapandı: %v", err)
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdown)
	}
}
