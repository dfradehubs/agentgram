package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/auth"
	"github.com/dfradehubs/agentgram-api/internal/config"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"go.uber.org/zap"
)

// Server represents the HTTP server
type Server struct {
	httpServer    *http.Server
	registry      *agents.Registry
	healthChecker *agents.HealthChecker
	closers       []io.Closer
	logger        *zap.Logger
	cfg           *config.Config
}

// New creates a new server
func New(cfg *config.Config, registry *agents.Registry, sessionStore store.SessionStore, authSessionStore store.AuthSessionStore, cookieCrypto *auth.CookieCrypto, logger *zap.Logger, adminDeps *AdminDeps, closers ...io.Closer) *Server {
	handler := SetupRoutes(cfg, registry, sessionStore, authSessionStore, cookieCrypto, logger, adminDeps)

	httpServer := &http.Server{
		Addr:        fmt.Sprintf(":%s", cfg.Server.Port),
		Handler:     handler,
		ReadTimeout: 15 * time.Second,
		// WriteTimeout disabled (0) for SSE: long-lived streams need unlimited write time.
		// Connection lifetime is controlled by the client (browser closes SSE on navigation/abort).
		IdleTimeout: 60 * time.Second,
	}

	return &Server{
		httpServer:    httpServer,
		registry:      registry,
		healthChecker: agents.NewHealthChecker(registry, logger),
		closers:       closers,
		logger:        logger,
		cfg:           cfg,
	}
}

// Start starts the server with graceful shutdown
func (s *Server) Start() error {
	// Start health checker
	s.healthChecker.Start()

	// Channel for shutdown signals
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Channel for server errors
	serverErrors := make(chan error, 1)

	// Start server in goroutine
	go func() {
		s.logger.Info("starting server",
			zap.String("addr", s.httpServer.Addr))
		serverErrors <- s.httpServer.ListenAndServe()
	}()

	// Wait for shutdown signal or error
	select {
	case err := <-serverErrors:
		if err != http.ErrServerClosed {
			return fmt.Errorf("server error: %w", err)
		}
	case sig := <-shutdown:
		s.logger.Info("shutdown signal received", zap.String("signal", sig.String()))
		return s.Shutdown()
	}

	return nil
}

// Shutdown performs a graceful shutdown of the server
func (s *Server) Shutdown() error {
	s.logger.Info("shutting down server...")

	// Create context with timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop health checker
	s.healthChecker.Stop()

	// Close additional resources (e.g., Redis client)
	for _, c := range s.closers {
		if err := c.Close(); err != nil {
			s.logger.Error("error closing resource", zap.Error(err))
		}
	}

	// HTTP server shutdown
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error("server shutdown error", zap.Error(err))
		return err
	}

	s.logger.Info("server stopped gracefully")
	return nil
}
