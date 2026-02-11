package batcha

import (
	"context"
	"fmt"
	"path/filepath"

	goconfig "github.com/kayac/go-config"
)

// render loads and renders the job definition template.
func (app *App) render(ctx context.Context) (map[string]any, error) {
	loader := goconfig.New()
	if err := setupPlugins(ctx, app.config, loader); err != nil {
		return nil, err
	}

	jobDefPath := app.config.JobDefinition
	if !filepath.IsAbs(jobDefPath) {
		jobDefPath = filepath.Join(filepath.Dir(app.configPath), jobDefPath)
	}

	var rendered map[string]any
	if err := loader.LoadWithEnvJSON(&rendered, jobDefPath); err != nil {
		return nil, fmt.Errorf("failed to render job definition template: %w", err)
	}
	return rendered, nil
}

// Render renders the job definition template and prints the result.
func (app *App) Render(ctx context.Context) error {
	return app.Register(ctx, RegisterOption{DryRun: true})
}
