package batcha

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// Config represents the batcha configuration file.
type Config struct {
	Region        string   `yaml:"region"`
	JobDefinition string   `yaml:"job_definition"`
	Plugins       []Plugin `yaml:"plugins"`
}

// Plugin represents a plugin configuration block.
type Plugin struct {
	Name   string       `yaml:"name"`
	Config PluginConfig `yaml:"config"`
}

// PluginConfig holds plugin-specific settings.
type PluginConfig struct {
	URL string `yaml:"url"`
}

// LoadConfig reads and validates the YAML config file.
func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	if cfg.JobDefinition == "" {
		return nil, fmt.Errorf("job_definition is required in config")
	}
	// Fallback to environment variables for region
	if cfg.Region == "" {
		cfg.Region = os.Getenv("AWS_REGION")
	}
	if cfg.Region == "" {
		cfg.Region = os.Getenv("AWS_DEFAULT_REGION")
	}
	return &cfg, nil
}
