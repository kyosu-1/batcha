package batcha

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

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
