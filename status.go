package batcha

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/batch"
)

// Status shows the current state of the job definition on AWS.
func (app *App) Status(ctx context.Context) error {
	rendered, err := app.render(ctx)
	if err != nil {
		return err
	}
	converted := walkMap(rendered, toPascalCase)

	name, _ := converted.(map[string]any)["JobDefinitionName"].(string)
	if name == "" {
		return fmt.Errorf("jobDefinitionName is required in job definition")
	}

	client, err := app.newBatchClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	out, err := client.DescribeJobDefinitions(ctx, &batch.DescribeJobDefinitionsInput{
		JobDefinitionName: aws.String(name),
		Status:            aws.String("ACTIVE"),
	})
	if err != nil {
		return fmt.Errorf("failed to describe job definitions: %w", err)
	}

	if len(out.JobDefinitions) == 0 {
		fmt.Printf("No active job definition found for %q.\n", name)
		return nil
	}

	latest := pickLatestRevision(out.JobDefinitions)

	fmt.Printf("Name:     %s\n", aws.ToString(latest.JobDefinitionName))
	fmt.Printf("ARN:      %s\n", aws.ToString(latest.JobDefinitionArn))
	fmt.Printf("Revision: %d\n", aws.ToInt32(latest.Revision))
	fmt.Printf("Status:   %s\n", aws.ToString(latest.Status))
	fmt.Printf("Type:     %s\n", aws.ToString(latest.Type))

	if cp := latest.ContainerProperties; cp != nil {
		fmt.Printf("Image:    %s\n", aws.ToString(cp.Image))
		for _, r := range cp.ResourceRequirements {
			fmt.Printf("%-9s %s\n", string(r.Type)+":", aws.ToString(r.Value))
		}
	}

	fmt.Printf("Active revisions: %d\n", len(out.JobDefinitions))
	return nil
}
