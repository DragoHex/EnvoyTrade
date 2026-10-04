package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"
)

// Server coordinates the entire testbroker service.
type Server struct {
	config     *Config
	engine     *OrderEngine
	hub        *WSHub
	postback   *PostbackDispatcher
	httpServer *http.Server
}

// NewServer initializes the OrderEngine, WSHub, PostbackDispatcher, and HTTP server.
func NewServer(cfg *Config) (*Server, error) {
	eng := NewOrderEngine(cfg)
	hub := NewWSHub(cfg)
	postback := NewPostbackDispatcher(cfg.PostbackURL, cfg.Users)

	// Wire terminal state hooks: dual notification (WS ticker + Postback webhook)
	eng.OnTerminal(func(o *Order) {
		hub.Broadcast(o)
		postback.Dispatch(o)
	})

	router := NewRouter(cfg, eng, hub, postback)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	return &Server{
		config:     cfg,
		engine:     eng,
		hub:        hub,
		postback:   postback,
		httpServer: httpServer,
	}, nil
}

// Start runs the HTTP server.
func (s *Server) Start() error {
	log.Printf("TestBroker listening on http://0.0.0.0:%d (UI: http://localhost:%d/ui)", s.config.Port, s.config.Port)
	log.Printf("WebSocket endpoint: ws://localhost:%d/?api_key=...&access_token=...", s.config.Port)
	log.Printf("Postback destination: %s", s.config.PostbackURL)
	log.Printf("Loaded %d users and %d instruments", len(s.config.Users), len(s.config.Instruments))

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("listen and serve: %w", err)
	}
	return nil
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
