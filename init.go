package batcha

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/batch"
	"gopkg.in/yaml.v2"
)

// InitOption holds options for the init command.
type InitOption struct {
	JobDefinitionName string
	Region            string
	OutputDir         string
}

// Init fetches an active job definition from AWS and generates config + template files.
func Init(ctx context.Context, opt InitOption) error {
	region := opt.Region
	if region == "" {
		region = os.Getenv("AWS_REGION")
	}
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}
	client := batch.NewFromConfig(awsCfg)

	out, err := client.DescribeJobDefinitions(ctx, &batch.DescribeJobDefinitionsInput{
		JobDefinitionName: aws.String(opt.JobDefinitionName),
		Status:            aws.String("ACTIVE"),
	})
	if err != nil {
		return fmt.Errorf("failed to describe job definitions: %w", err)
	}
	if len(out.JobDefinitions) == 0 {
		return fmt.Errorf("no active job definition found for %q", opt.JobDefinitionName)
	}

	latest := pickLatestRevision(out.JobDefinitions)

	// Marshal to JSON then back to map[string]any to get a clean structure
	jsonBytes, err := json.Marshal(latest)
	if err != nil {
		return fmt.Errorf("failed to marshal job definition: %w", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(jsonBytes, &raw); err != nil {
		return fmt.Errorf("failed to unmarshal job definition: %w", err)
	}

	// Remove AWS-managed fields that shouldn't be in a template
	for _, key := range initExcludeKeys {
		delete(raw, key)
	}

	// Convert PascalCase to camelCase
	converted := walkMap(raw, toCamelCase)

	formatted, err := json.MarshalIndent(converted, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format job definition: %w", err)
	}

	// Write job-definition.json
	jobDefPath := filepath.Join(opt.OutputDir, "job-definition.json")
	if err := os.WriteFile(jobDefPath, append(formatted, '\n'), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", jobDefPath, err)
	}
	fmt.Printf("Created %s\n", jobDefPath)

	// Write batcha.yml
	cfg := Config{
		Region:        region,
		JobDefinition: "job-definition.json",
	}
	cfgBytes, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	cfgPath := filepath.Join(opt.OutputDir, "batcha.yml")
	if err := os.WriteFile(cfgPath, cfgBytes, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", cfgPath, err)
	}
	fmt.Printf("Created %s\n", cfgPath)

	return nil
}

// initExcludeKeys are fields returned by DescribeJobDefinitions that are
// AWS-managed and should not be included in a user-managed template.
var initExcludeKeys = []string{
	"JobDefinitionArn",
	"Revision",
	"Status",
	"ContainerOrchestrationType",
}
