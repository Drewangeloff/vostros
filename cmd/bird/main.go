package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	root "github.com/drewangeloff/old_school_bird"
	"github.com/drewangeloff/old_school_bird/internal/auth"
	"github.com/drewangeloff/old_school_bird/internal/config"
	"github.com/drewangeloff/old_school_bird/internal/fanout"
	"github.com/drewangeloff/old_school_bird/internal/handler"
	"github.com/drewangeloff/old_school_bird/internal/moderation"
	"github.com/drewangeloff/old_school_bird/internal/ratelimit"
	"github.com/drewangeloff/old_school_bird/internal/repository"
	"github.com/drewangeloff/old_school_bird/internal/router"
	"github.com/drewangeloff/old_school_bird/internal/tmpl"
)

func main() {
	cfg := config.Load()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect to Postgres
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connecting to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("pinging database: %v", err)
	}
	log.Println("connected to database")

	// Run migrations
	if err := repository.RunMigrations(ctx, pool, root.MigrationsFS); err != nil {
		log.Fatalf("running migrations: %v", err)
	}

	// Build dependencies
	repo := repository.NewPostgres(pool)
	authService := auth.NewService(cfg.JWTSecret)
	renderer := tmpl.New(root.TemplateFS, cfg.DevMode)
	mod := moderation.NewRegexModerator()
	h := handler.New(repo, renderer, authService, mod)
	authMiddleware := auth.NewMiddleware(authService, repo)
	limiter := ratelimit.New(100, time.Minute) // 100 requests per minute per IP
	mux := router.New(h, root.StaticFS, authMiddleware, limiter)

	// Start outbox processor
	outboxProcessor := fanout.New(repo, 2*time.Second)
	go outboxProcessor.Start(ctx)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-done
	log.Println("shutting down...")
	cancel() // stops outbox processor

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	log.Println("goodbye")
}
