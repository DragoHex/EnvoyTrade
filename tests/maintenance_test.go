package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPostgresConfigParameters(t *testing.T) {
	// Find root directory
	rootDir, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	confPath := filepath.Join(rootDir, "packaging", "postgres", "envoytrade-postgres.conf")
	data, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("failed to read envoytrade-postgres.conf: %v", err)
	}

	content := string(data)

	expectedSettings := []string{
		"listen_addresses = '*'",
		"max_connections = 100",
		"shared_buffers = 1GB",
		"work_mem = 32MB",
		"maintenance_work_mem = 128MB",
		"effective_cache_size = 3GB",
		"synchronous_commit = on",
		"autovacuum = on",
		"autovacuum_vacuum_scale_factor = 0.05",
		"autovacuum_analyze_scale_factor = 0.02",
		"autovacuum_vacuum_cost_limit = 500",
		"autovacuum_vacuum_cost_delay = 2ms",
	}

	for _, setting := range expectedSettings {
		if !strings.Contains(content, setting) {
			t.Errorf("missing expected setting in envoytrade-postgres.conf: %q", setting)
		}
	}
}

func TestMaintenanceCronSchedule(t *testing.T) {
	rootDir, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	cronPath := filepath.Join(rootDir, "packaging", "cron", "envoytrade-maintenance.cron")
	data, err := os.ReadFile(cronPath)
	if err != nil {
		t.Fatalf("failed to read envoytrade-maintenance.cron: %v", err)
	}

	content := string(data)

	// Check for 1:00 AM IST requirement (either 0 1 * * * IST or 30 20 * * * UTC)
	if !strings.Contains(content, "0 1 * * *") && !strings.Contains(content, "30 20 * * *") {
		t.Errorf("expected cron file to contain 1:00 AM IST trigger schedule (0 1 * * * or 30 20 * * *), got:\n%s", content)
	}

	// Must invoke envoytrade-db-maintenance or db_maintenance.sh
	if !strings.Contains(content, "envoytrade-db-maintenance") {
		t.Errorf("expected cron to run envoytrade-db-maintenance binary/script")
	}
}

func TestMaintenanceScriptDryRun(t *testing.T) {
	rootDir, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	scriptPath := filepath.Join(rootDir, "scripts", "db_maintenance.sh")
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("scripts/db_maintenance.sh not found: %v", err)
	}

	// Check executable permission
	if info.Mode()&0111 == 0 {
		t.Errorf("scripts/db_maintenance.sh is not executable")
	}

	// Execute with --dry-run
	cmd := exec.Command(scriptPath, "--dry-run")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to execute db_maintenance.sh --dry-run: %v\nOutput: %s", err, string(out))
	}

	outStr := string(out)
	if !strings.Contains(outStr, "DRY-RUN") && !strings.Contains(outStr, "dry-run") {
		t.Errorf("expected dry-run output to indicate DRY-RUN mode, got:\n%s", outStr)
	}
	if !strings.Contains(outStr, "VACUUM ANALYZE") {
		t.Errorf("expected dry-run output to mention VACUUM ANALYZE, got:\n%s", outStr)
	}
}
