package testbroker

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"envoytrade/internal/domain"
	"envoytrade/internal/queue/memchan"

	"github.com/google/uuid"
)

func TestTickerFactory_CreateTicker(t *testing.T) {
	q := memchan.New[domain.MasterFill](16)
	logger := slog.Default()

	factory := NewTickerFactory(q, logger)

	masterID := uuid.New()
	authInfo := domain.AccountAuthInfo{
		Broker:          "testbroker",
		BrokerAccountID: "MASTER01",
		ApiKey:          "key_master",
		AccessToken:     "token_master",
	}

	ticker, err := factory.CreateTicker(context.Background(), masterID, authInfo, nil)
	if err != nil {
		t.Fatalf("CreateTicker failed: %v", err)
	}
	if ticker == nil {
		t.Fatal("expected non-nil ticker")
	}
}

func TestTickerFactory_MissingCredentials(t *testing.T) {
	q := memchan.New[domain.MasterFill](16)
	factory := NewTickerFactory(q, slog.Default())

	ticker, err := factory.CreateTicker(context.Background(), uuid.New(), domain.AccountAuthInfo{
		Broker: "testbroker",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ticker != nil {
		t.Fatal("expected nil ticker when credentials are empty")
	}
}

func TestTickerFactory_CustomURL(t *testing.T) {
	q := memchan.New[domain.MasterFill](16)
	factory := NewTickerFactory(q, slog.Default())

	t.Run("Default Root URL", func(t *testing.T) {
		os.Unsetenv("TESTBROKER_WS_URL")
		f := NewTickerFactory(q, slog.Default())
		if f.WSRootURL() != "ws://localhost:8089" {
			t.Errorf("WSRootURL = %s, want ws://localhost:8089", f.WSRootURL())
		}
	})

	t.Run("Env Override", func(t *testing.T) {
		t.Setenv("TESTBROKER_WS_URL", "ws://custom-host:9999")
		f := NewTickerFactory(q, slog.Default())
		if f.WSRootURL() != "ws://custom-host:9999" {
			t.Errorf("WSRootURL = %s, want ws://custom-host:9999", f.WSRootURL())
		}
	})

	t.Run("Manual Setter", func(t *testing.T) {
		factory.SetWSRootURL("ws://override:8000")
		if factory.WSRootURL() != "ws://override:8000" {
			t.Errorf("WSRootURL = %s, want ws://override:8000", factory.WSRootURL())
		}
	})
}
