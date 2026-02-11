package batcha

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/batch"
	batchTypes "github.com/aws/aws-sdk-go-v2/service/batch/types"
)

// RunOption holds options for the run command.
type RunOption struct {
	JobQueue   string
	JobName    string
	Parameters map[string]string
	Wait       bool
}

// Run submits a job using the latest active job definition.
func (app *App) Run(ctx context.Context, opt RunOption) error {
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

	// Fetch the latest active revision ARN
	out, err := client.DescribeJobDefinitions(ctx, &batch.DescribeJobDefinitionsInput{
		JobDefinitionName: aws.String(name),
		Status:            aws.String("ACTIVE"),
	})
	if err != nil {
		return fmt.Errorf("failed to describe job definitions: %w", err)
	}
	if len(out.JobDefinitions) == 0 {
		return fmt.Errorf("no active job definition found for %q", name)
	}
	latest := pickLatestRevision(out.JobDefinitions)

	jobName := opt.JobName
	if jobName == "" {
		jobName = name
	}

	input := &batch.SubmitJobInput{
		JobDefinition: latest.JobDefinitionArn,
		JobQueue:      aws.String(opt.JobQueue),
		JobName:       aws.String(jobName),
	}
	if len(opt.Parameters) > 0 {
		input.Parameters = opt.Parameters
	}

	result, err := client.SubmitJob(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to submit job: %w", err)
	}

	fmt.Printf("Submitted job: %s (ID: %s)\n", aws.ToString(result.JobName), aws.ToString(result.JobId))

	if !opt.Wait {
		return nil
	}

	return app.waitForJob(ctx, client, aws.ToString(result.JobId))
}

func (app *App) waitForJob(ctx context.Context, client *batch.Client, jobID string) error {
	fmt.Printf("Waiting for job %s...\n", jobID)

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	var lastStatus batchTypes.JobStatus
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			out, err := client.DescribeJobs(ctx, &batch.DescribeJobsInput{
				Jobs: []string{jobID},
			})
			if err != nil {
				return fmt.Errorf("failed to describe job: %w", err)
			}
			if len(out.Jobs) == 0 {
				return fmt.Errorf("job %s not found", jobID)
			}

			job := out.Jobs[0]
			if job.Status != lastStatus {
				fmt.Printf("  %s\n", job.Status)
				lastStatus = job.Status
			}

			switch job.Status {
			case batchTypes.JobStatusSucceeded:
				fmt.Println("Job succeeded.")
				return nil
			case batchTypes.JobStatusFailed:
				reason := aws.ToString(job.StatusReason)
				return fmt.Errorf("job failed: %s", reason)
			}
		}
	}
}
