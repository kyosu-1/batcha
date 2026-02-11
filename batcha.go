package batcha

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/batch"
	batchTypes "github.com/aws/aws-sdk-go-v2/service/batch/types"
	"github.com/fujiwara/tfstate-lookup/tfstate"
	goconfig "github.com/kayac/go-config"
	"gopkg.in/yaml.v2"
)

// Version is set by goreleaser via ldflags.
var Version = "dev"

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

// RegisterOption holds options for the register command.
type RegisterOption struct {
	DryRun bool
}

// Register renders and registers the job definition with AWS Batch.
func (app *App) Register(ctx context.Context, opt RegisterOption) error {
	rendered, err := app.render(ctx)
	if err != nil {
		return err
	}

	converted := walkMap(rendered, toPascalCase)

	jsonBytes, err := json.Marshal(converted)
	if err != nil {
		return fmt.Errorf("failed to marshal job definition: %w", err)
	}

	if opt.DryRun {
		formatted, err := json.MarshalIndent(json.RawMessage(jsonBytes), "", "  ")
		if err != nil {
			return fmt.Errorf("failed to format JSON: %w", err)
		}
		fmt.Println(string(formatted))
		return nil
	}

	var input batch.RegisterJobDefinitionInput
	if err := json.Unmarshal(jsonBytes, &input); err != nil {
		return fmt.Errorf("failed to unmarshal into RegisterJobDefinitionInput: %w", err)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(app.config.Region))
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}
	client := batch.NewFromConfig(awsCfg)

	result, err := client.RegisterJobDefinition(ctx, &input)
	if err != nil {
		return fmt.Errorf("failed to register job definition: %w", err)
	}

	fmt.Printf("Registered: %s revision %d\n",
		aws.ToString(result.JobDefinitionName),
		aws.ToInt32(result.Revision),
	)
	return nil
}

// Render renders the job definition template and prints the result.
func (app *App) Render(ctx context.Context) error {
	return app.Register(ctx, RegisterOption{DryRun: true})
}

// Diff compares the local rendered definition with the active one on AWS.
// Returns an error wrapping ErrDiffFound if differences exist (exit code 1 for CI).
func (app *App) Diff(ctx context.Context) error {
	rendered, err := app.render(ctx)
	if err != nil {
		return err
	}
	converted := walkMap(rendered, toPascalCase)
	localBytes, err := json.MarshalIndent(converted, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal local definition: %w", err)
	}

	// Extract job definition name from the rendered template
	name, _ := converted.(map[string]any)["JobDefinitionName"].(string)
	if name == "" {
		return fmt.Errorf("jobDefinitionName is required in job definition")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(app.config.Region))
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}
	client := batch.NewFromConfig(awsCfg)

	out, err := client.DescribeJobDefinitions(ctx, &batch.DescribeJobDefinitionsInput{
		JobDefinitionName: aws.String(name),
		Status:            aws.String("ACTIVE"),
	})
	if err != nil {
		return fmt.Errorf("failed to describe job definitions: %w", err)
	}

	if len(out.JobDefinitions) == 0 {
		fmt.Printf("No active job definition found for %q. The local definition will be newly registered.\n", name)
		fmt.Println(string(localBytes))
		return &DiffError{}
	}

	// Pick the latest revision
	latest := pickLatestRevision(out.JobDefinitions)

	remoteBytes, err := json.MarshalIndent(latest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal remote definition: %w", err)
	}

	diff := unifiedDiff(string(remoteBytes), string(localBytes), "remote", "local")
	if diff == "" {
		fmt.Println("No differences found.")
		return nil
	}
	fmt.Println(diff)
	return &DiffError{}
}

// DiffError is returned when diff finds differences.
type DiffError struct{}

func (e *DiffError) Error() string { return "differences found" }

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

// --- JSON utilities ---

// skipPascalKeys are map keys that should NOT be converted to PascalCase
// because they are user-defined (e.g., environment variable names, tags).
var skipPascalKeys = map[string]bool{
	"options":    true,
	"parameters": true,
	"tags":       true,
}

// walkMap recursively converts map keys using the provided function.
func walkMap(v any, fn func(string) string) any {
	switch val := v.(type) {
	case map[string]any:
		result := make(map[string]any, len(val))
		for k, child := range val {
			newKey := fn(k)
			if skipPascalKeys[k] {
				result[newKey] = child
			} else {
				result[newKey] = walkMap(child, fn)
			}
		}
		return result
	case []any:
		result := make([]any, len(val))
		for i, child := range val {
			result[i] = walkMap(child, fn)
		}
		return result
	default:
		return v
	}
}

// toPascalCase converts a camelCase string to PascalCase.
func toPascalCase(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// --- Unified diff (stdlib only) ---

// unifiedDiff produces a unified diff string between two texts.
// Returns an empty string if there are no differences.
func unifiedDiff(a, b, labelA, labelB string) string {
	linesA := strings.Split(a, "\n")
	linesB := strings.Split(b, "\n")

	// Simple LCS-based diff
	lcs := lcsTable(linesA, linesB)
	hunks := buildHunks(linesA, linesB, lcs)
	if len(hunks) == 0 {
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n", labelA)
	fmt.Fprintf(&sb, "+++ %s\n", labelB)
	for _, h := range hunks {
		sb.WriteString(h)
	}
	return sb.String()
}

func lcsTable(a, b []string) [][]int {
	m, n := len(a), len(b)
	table := make([][]int, m+1)
	for i := range table {
		table[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if a[i] == b[j] {
				table[i][j] = table[i+1][j+1] + 1
			} else if table[i+1][j] >= table[i][j+1] {
				table[i][j] = table[i+1][j]
			} else {
				table[i][j] = table[i][j+1]
			}
		}
	}
	return table
}

type diffOp struct {
	kind byte // ' ', '-', '+'
	line string
	posA int
	posB int
}

func buildHunks(a, b []string, lcs [][]int) []string {
	ops := buildOps(a, b, lcs)
	if len(ops) == 0 {
		return nil
	}

	// Check if there are any actual changes
	hasChanges := false
	for _, op := range ops {
		if op.kind != ' ' {
			hasChanges = true
			break
		}
	}
	if !hasChanges {
		return nil
	}

	const ctx = 3
	var hunks []string
	var hunkOps []diffOp
	lastChange := -1

	for i, op := range ops {
		if op.kind != ' ' {
			if lastChange == -1 {
				// Start new hunk with context
				start := max(i-ctx, 0)
				hunkOps = ops[start:i]
			} else if i-lastChange > 2*ctx {
				// Flush previous hunk
				end := min(lastChange+ctx+1, len(ops))
				hunks = append(hunks, formatHunk(append(hunkOps, ops[lastChange+1:end]...)))
				// Start new hunk
				start := max(i-ctx, 0)
				hunkOps = make([]diffOp, 0)
				for j := start; j < i; j++ {
					hunkOps = append(hunkOps, ops[j])
				}
			} else {
				// Continue current hunk
				hunkOps = append(hunkOps, ops[lastChange+1:i]...)
			}
			hunkOps = append(hunkOps, op)
			lastChange = i
		}
	}

	// Flush final hunk
	if lastChange >= 0 {
		end := min(lastChange+ctx+1, len(ops))
		hunks = append(hunks, formatHunk(append(hunkOps, ops[lastChange+1:end]...)))
	}

	return hunks
}

func buildOps(a, b []string, lcs [][]int) []diffOp {
	var ops []diffOp
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i] == b[j] {
			ops = append(ops, diffOp{' ', a[i], i, j})
			i++
			j++
		} else if lcs[i+1][j] >= lcs[i][j+1] {
			ops = append(ops, diffOp{'-', a[i], i, j})
			i++
		} else {
			ops = append(ops, diffOp{'+', b[j], i, j})
			j++
		}
	}
	for ; i < len(a); i++ {
		ops = append(ops, diffOp{'-', a[i], i, j})
	}
	for ; j < len(b); j++ {
		ops = append(ops, diffOp{'+', b[j], i, j})
	}
	return ops
}

func formatHunk(ops []diffOp) string {
	if len(ops) == 0 {
		return ""
	}
	startA, startB := ops[0].posA+1, ops[0].posB+1
	countA, countB := 0, 0
	for _, op := range ops {
		switch op.kind {
		case ' ':
			countA++
			countB++
		case '-':
			countA++
		case '+':
			countB++
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", startA, countA, startB, countB)
	for _, op := range ops {
		fmt.Fprintf(&sb, "%c%s\n", op.kind, op.line)
	}
	return sb.String()
}
