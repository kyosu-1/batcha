package batcha

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/batch"
)

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

	client, err := app.newBatchClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Check if the remote definition already matches
	name, _ := converted.(map[string]any)["JobDefinitionName"].(string)
	if name != "" {
		out, err := client.DescribeJobDefinitions(ctx, &batch.DescribeJobDefinitionsInput{
			JobDefinitionName: aws.String(name),
			Status:            aws.String("ACTIVE"),
		})
		if err == nil && len(out.JobDefinitions) > 0 {
			latest := pickLatestRevision(out.JobDefinitions)
			remoteMap, err := normalizeRemoteDefinition(latest)
			if err == nil && reflect.DeepEqual(remoteMap, converted) {
				fmt.Printf("No changes detected. Skip registration. (current revision: %d)\n", aws.ToInt32(latest.Revision))
				return nil
			}
		}
	}

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
