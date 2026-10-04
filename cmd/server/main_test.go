package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaskDatabaseURL(t *testing.T) {
	raw := "postgres://user:secretpassword@localhost:5432/mydb?sslmode=disable"
	masked := maskDatabaseURL(raw)
	if strings.Contains(masked, "secretpassword") {
		t.Fatalf("expected password to be masked, got: %s", masked)
	}
	if !strings.Contains(masked, "user") || !strings.Contains(masked, "localhost:5432") || !strings.Contains(masked, "mydb") {
		t.Fatalf("unexpected masked url: %s", masked)
	}
}

func TestSetupLogger_FileCreationAndFallback(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "logs", "test.log")

	t.Setenv("LOG_FILE", logFile)
	t.Setenv("LOG_FORMAT", "json")
	t.Setenv("LOG_LEVEL", "debug")

	logger, cleanup, err := setupLogger()
	if err != nil {
		t.Fatalf("setupLogger failed: %v", err)
	}

	logger.Info("test message", "key", "val")
	cleanup()

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	if !strings.Contains(string(data), "test message") {
		t.Errorf("log file missing message: %s", string(data))
	}
	if !strings.Contains(string(data), `"key":"val"`) {
		t.Errorf("log file missing attributes: %s", string(data))
	}
}

func TestSetupLogger_FallbackWhenDirUnwritable(t *testing.T) {
	// A path in an unwritable directory falls back to stdout
	t.Setenv("LOG_FILE", "/unwritable_root_dir_test_1234/test.log")
	t.Setenv("LOG_FORMAT", "text")

	logger, cleanup, err := setupLogger()
	if err != nil {
		t.Fatalf("setupLogger unexpected error: %v", err)
	}
	defer cleanup()

	if logger == nil {
		t.Fatal("expected fallback logger, got nil")
	}
}

func TestRun_RequiresEnvVars(t *testing.T) {
	logger := slog.Default()

	// Missing DATABASE_URL
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ENCRYPTION_KEY", "any-key")
	err := run(logger)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Fatalf("expected DATABASE_URL is required error, got: %v", err)
	}

	// Missing ENCRYPTION_KEY
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/db")
	t.Setenv("ENCRYPTION_KEY", "")
	err = run(logger)
	if err == nil || !strings.Contains(err.Error(), "ENCRYPTION_KEY is required") {
		t.Fatalf("expected ENCRYPTION_KEY is required error, got: %v", err)
	}
}

