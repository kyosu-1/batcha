package batcha

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/batch"
	batchTypes "github.com/aws/aws-sdk-go-v2/service/batch/types"
	"github.com/fujiwara/tfstate-lookup/tfstate"
	goconfig "github.com/kayac/go-config"
)

// Version is set by goreleaser via ldflags.
var Version = "dev"

// App is the main application struct.
type App struct {
	config     *Config
	configPath string
}

// New creates a new App by loading the config file.
func New(ctx context.Context, configPath string) (*App, error) {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	return &App{config: cfg, configPath: configPath}, nil
}

// newBatchClient creates an AWS Batch client from the app's config region.
func (app *App) newBatchClient(ctx context.Context) (*batch.Client, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(app.config.Region))
	if err != nil {
		return nil, err
	}
	return batch.NewFromConfig(awsCfg), nil
}

// setupPlugins configures the go-config loader with tfstate FuncMaps.
func setupPlugins(ctx context.Context, cfg *Config, loader *goconfig.Loader) error {
	for _, p := range cfg.Plugins {
		if p.Name != "tfstate" {
			continue
		}
		funcMap, err := tfstate.FuncMap(ctx, p.Config.URL)
		if err != nil {
			return fmt.Errorf("failed to load tfstate from %s: %w", p.Config.URL, err)
		}
		loader.Funcs(funcMap)
	}
	return nil
}

// pickLatestRevision returns the job definition with the highest revision.
func pickLatestRevision(defs []batchTypes.JobDefinition) batchTypes.JobDefinition {
	latest := defs[0]
	for _, d := range defs[1:] {
		if aws.ToInt32(d.Revision) > aws.ToInt32(latest.Revision) {
			latest = d
		}
	}
	return latest
}

// normalizeRemoteDefinition converts an AWS job definition to a comparable
// map by stripping AWS-managed fields.
func normalizeRemoteDefinition(def batchTypes.JobDefinition) (map[string]any, error) {
	b, err := json.Marshal(def)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal remote definition: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("failed to unmarshal remote definition: %w", err)
	}
	for _, key := range initExcludeKeys {
		delete(m, key)
	}
	return m, nil
}
