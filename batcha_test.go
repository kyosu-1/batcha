package batcha

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- toPascalCase tests ---

func TestToPascalCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"a", "A"},
		{"jobDefinitionName", "JobDefinitionName"},
		{"type", "Type"},
		{"containerProperties", "ContainerProperties"},
		{"VCPU", "VCPU"},
		{"Already", "Already"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toPascalCase(tt.input)
			if got != tt.want {
				t.Errorf("toPascalCase(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- walkMap tests ---

func TestWalkMap(t *testing.T) {
	input := map[string]any{
		"jobDefinitionName": "test-job",
		"containerProperties": map[string]any{
			"image": "nginx",
			"environment": []any{
				map[string]any{
					"name":  "FOO",
					"value": "bar",
				},
			},
		},
		"tags": map[string]any{
			"myTag": "value",
		},
		"parameters": map[string]any{
			"inputFile": "s3://bucket/file",
		},
	}

	result := walkMap(input, toPascalCase).(map[string]any)

	// Top-level keys should be PascalCase
	if _, ok := result["JobDefinitionName"]; !ok {
		t.Error("expected JobDefinitionName key")
	}

	// Nested keys should be PascalCase
	cp := result["ContainerProperties"].(map[string]any)
	if _, ok := cp["Image"]; !ok {
		t.Error("expected Image key in ContainerProperties")
	}

	// Environment inside array should be PascalCase
	envList := cp["Environment"].([]any)
	envItem := envList[0].(map[string]any)
	if _, ok := envItem["Name"]; !ok {
		t.Error("expected Name key in environment item")
	}

	// Tags keys should NOT be converted (skipPascalKeys)
	tags := result["Tags"].(map[string]any)
	if _, ok := tags["myTag"]; !ok {
		t.Error("expected tags keys to be preserved as-is")
	}

	// Parameters keys should NOT be converted (skipPascalKeys)
	params := result["Parameters"].(map[string]any)
	if _, ok := params["inputFile"]; !ok {
		t.Error("expected parameters keys to be preserved as-is")
	}
}

// --- LoadConfig tests ---

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

// --- render tests ---

func TestRender(t *testing.T) {
	t.Setenv("TEST_JOB_NAME", "my-job")
	t.Setenv("TEST_IMAGE", "myrepo/myimage:v1")
	t.Setenv("APP_ENV", "staging")

	app, err := New(context.Background(), filepath.Join("testdata", "config.yml"))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	rendered, err := app.render(context.Background())
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	name, ok := rendered["jobDefinitionName"].(string)
	if !ok || name != "my-job" {
		t.Errorf("jobDefinitionName = %v, want %q", rendered["jobDefinitionName"], "my-job")
	}

	cp := rendered["containerProperties"].(map[string]any)
	image, ok := cp["image"].(string)
	if !ok || image != "myrepo/myimage:v1" {
		t.Errorf("image = %v, want %q", cp["image"], "myrepo/myimage:v1")
	}
}

func TestRender_DefaultEnv(t *testing.T) {
	// Unset env vars to test defaults
	t.Setenv("TEST_JOB_NAME", "")
	if err := os.Unsetenv("TEST_JOB_NAME"); err != nil {
		t.Fatal(err)
	}

	app, err := New(context.Background(), filepath.Join("testdata", "config.yml"))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	rendered, err := app.render(context.Background())
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	name := rendered["jobDefinitionName"].(string)
	if name != "example-job" {
		t.Errorf("jobDefinitionName = %q, want %q (default)", name, "example-job")
	}
}

// --- DryRun (register --dry-run) test ---

func TestRegister_DryRun(t *testing.T) {
	t.Setenv("TEST_JOB_NAME", "dry-run-job")

	app, err := New(context.Background(), filepath.Join("testdata", "config.yml"))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// dry-run just prints JSON and returns nil
	err = app.Register(context.Background(), RegisterOption{DryRun: true})
	if err != nil {
		t.Fatalf("Register dry-run failed: %v", err)
	}
}

// --- unifiedDiff tests ---

func TestUnifiedDiff_NoDiff(t *testing.T) {
	a := "line1\nline2\nline3"
	b := "line1\nline2\nline3"
	diff := unifiedDiff(a, b, "a", "b")
	if diff != "" {
		t.Errorf("expected empty diff, got:\n%s", diff)
	}
}

func TestUnifiedDiff_WithChanges(t *testing.T) {
	a := "line1\nline2\nline3"
	b := "line1\nmodified\nline3"
	diff := unifiedDiff(a, b, "a", "b")
	if diff == "" {
		t.Error("expected non-empty diff")
	}
	// Should contain unified diff markers
	if !strings.Contains(diff, "---") || !strings.Contains(diff, "+++") || !strings.Contains(diff, "@@") {
		t.Errorf("diff missing markers:\n%s", diff)
	}
	if !strings.Contains(diff, "-line2") || !strings.Contains(diff, "+modified") {
		t.Errorf("diff missing expected lines:\n%s", diff)
	}
}
