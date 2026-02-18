// TEACHING NOTES:
// This is the process entrypoint for the CMS binary.
// Go starts execution from main.main(), so this file wires startup concerns
// (configuration, app construction, HTTP server setup, and graceful shutdown).
// Useful Go concepts to notice while reading:
// 1. `context.Context` carries cancellation/deadline signals across call chains.
// 2. `defer` is used to guarantee cleanup (like closing DB connections).
// 3. `http.Server` is configured explicitly for timeouts/security defaults.
// 4. Fatal startup errors should fail fast rather than letting the app limp along.
// 5. Small helper functions keep `main()` readable and testable.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kcnotes/internal/app"
)

// main explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func main() {
	cfg := app.LoadConfig()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	mode := flag.String("mode", "serve", "mode: serve|migrate|create-user|publish|preview")
	userEmail := flag.String("email", "", "email for create-user mode")
	userPassword := flag.String("password", "", "password for create-user mode")
	userRole := flag.String("role", "admin", "role for create-user mode: admin|editor|author")
	flag.Parse()

	application, err := app.New(cfg, logger)
	if err != nil {
		logger.Error("initialize app", "error", err)
		os.Exit(1)
	}
	defer application.Close()

	switch *mode {
	case "migrate":
		if err := application.Migrate(context.Background()); err != nil {
			logger.Error("run migrations", "error", err)
			os.Exit(1)
		}
		logger.Info("migrations applied")
	case "create-user":
		if *userEmail == "" || *userPassword == "" {
			logger.Error("email and password are required for create-user mode")
			os.Exit(1)
		}
		if err := application.CreateUser(context.Background(), *userEmail, *userPassword, *userRole); err != nil {
			logger.Error("create user", "error", err)
			os.Exit(1)
		}
		logger.Info("user created", "email", *userEmail, "role", *userRole)
	case "serve":
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		srv := &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           application.Router(),
			ReadTimeout:       10 * time.Second,
			ReadHeaderTimeout: 5 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       60 * time.Second,
			MaxHeaderBytes:    8 << 10,
		}

		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
		}()

		logger.Info("starting server", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server exited", "error", err)
			os.Exit(1)
		}
	case "publish":
		result, err := application.Publish(context.Background())
		if err != nil {
			logger.Error("publish static site", "error", err)
			os.Exit(1)
		}
		logger.Info("publish completed", "out_dir", cfg.PublishOutDir, "generated_files", result.GeneratedFiles, "removed_files", result.RemovedFiles, "include_drafts", cfg.PublishIncludeDrafts)
	case "preview":
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		srv := &http.Server{
			Addr:              cfg.PreviewHTTPAddr,
			Handler:           http.FileServer(http.Dir(cfg.PublishOutDir)),
			ReadTimeout:       10 * time.Second,
			ReadHeaderTimeout: 5 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       60 * time.Second,
			MaxHeaderBytes:    8 << 10,
		}
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
		}()

		logger.Info("starting static preview server", "addr", cfg.PreviewHTTPAddr, "dir", cfg.PublishOutDir)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("preview server exited", "error", err)
			os.Exit(1)
		}
	default:
		logger.Error("unknown mode", "mode", *mode)
		os.Exit(1)
	}
}
