package batcha

import (
	"context"
	"path/filepath"
	"testing"
)

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
