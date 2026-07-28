package config

import (
	"testing"
)

// setMinimalEnv sets the required env vars so Load() doesn't fail validation.
func setMinimalEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DASHSCOPE_API_KEY", "test-key")
	t.Setenv("DASHSCOPE_EMBEDDING_API_KEY", "test-embed-key")
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
}

func TestLoad_QueueDefaults(t *testing.T) {
	setMinimalEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.QueueConcurrency != 10 {
		t.Fatalf("QueueConcurrency: expected 10, got %d", cfg.QueueConcurrency)
	}
	if cfg.QueueMaxRetry != 3 {
		t.Fatalf("QueueMaxRetry: expected 3, got %d", cfg.QueueMaxRetry)
	}
	if cfg.QueueIngestionWeight != 3 {
		t.Fatalf("QueueIngestionWeight: expected 3, got %d", cfg.QueueIngestionWeight)
	}
	if cfg.QueueReportWeight != 2 {
		t.Fatalf("QueueReportWeight: expected 2, got %d", cfg.QueueReportWeight)
	}
	if cfg.QueueDeliveryWeight != 1 {
		t.Fatalf("QueueDeliveryWeight: expected 1, got %d", cfg.QueueDeliveryWeight)
	}
	if cfg.MaxActiveJobsPerTenant != 3 {
		t.Fatalf("MaxActiveJobsPerTenant: expected 3, got %d", cfg.MaxActiveJobsPerTenant)
	}
}

func TestLoad_QueueFromEnv(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("QUEUE_CONCURRENCY", "20")
	t.Setenv("QUEUE_MAX_RETRY", "5")
	t.Setenv("QUEUE_INGESTION_WEIGHT", "6")
	t.Setenv("QUEUE_REPORT_WEIGHT", "4")
	t.Setenv("QUEUE_DELIVERY_WEIGHT", "2")
	t.Setenv("MAX_ACTIVE_JOBS_PER_TENANT", "10")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.QueueConcurrency != 20 {
		t.Fatalf("QueueConcurrency: expected 20, got %d", cfg.QueueConcurrency)
	}
	if cfg.QueueMaxRetry != 5 {
		t.Fatalf("QueueMaxRetry: expected 5, got %d", cfg.QueueMaxRetry)
	}
	if cfg.QueueIngestionWeight != 6 {
		t.Fatalf("QueueIngestionWeight: expected 6, got %d", cfg.QueueIngestionWeight)
	}
	if cfg.QueueReportWeight != 4 {
		t.Fatalf("QueueReportWeight: expected 4, got %d", cfg.QueueReportWeight)
	}
	if cfg.QueueDeliveryWeight != 2 {
		t.Fatalf("QueueDeliveryWeight: expected 2, got %d", cfg.QueueDeliveryWeight)
	}
	if cfg.MaxActiveJobsPerTenant != 10 {
		t.Fatalf("MaxActiveJobsPerTenant: expected 10, got %d", cfg.MaxActiveJobsPerTenant)
	}
}
