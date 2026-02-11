package batcha

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join("testdata", "config.yml"))
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Region != "ap-northeast-1" {
		t.Errorf("Region = %q, want %q", cfg.Region, "ap-northeast-1")
	}
	if cfg.JobDefinition != "job-definition.json" {
		t.Errorf("JobDefinition = %q, want %q", cfg.JobDefinition, "job-definition.json")
	}
}

func TestLoadConfig_RegionFallback(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(cfgPath, []byte("job_definition: job.json\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "job.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("AWS_REGION", "us-west-2")
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Region != "us-west-2" {
		t.Errorf("Region = %q, want %q", cfg.Region, "us-west-2")
	}
}

func TestLoadConfig_JobQueue(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(cfgPath, []byte("region: us-east-1\njob_definition: job.json\njob_queue: my-queue\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "job.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.JobQueue != "my-queue" {
		t.Errorf("JobQueue = %q, want %q", cfg.JobQueue, "my-queue")
	}
}

func TestLoadConfig_MissingJobDefinition(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(cfgPath, []byte("region: us-east-1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing job_definition")
	}
}
