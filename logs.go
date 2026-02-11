package batcha

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/batch"
	batchTypes "github.com/aws/aws-sdk-go-v2/service/batch/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
)

// LogsOption holds options for the logs command.
type LogsOption struct {
	JobID    string
	JobQueue string
	Follow   bool
	Since    time.Duration
}

// Logs fetches and displays CloudWatch logs for a Batch job.
func (app *App) Logs(ctx context.Context, opt LogsOption) error {
	// Resolve job queue: CLI flag > config
	if opt.JobQueue == "" {
		opt.JobQueue = app.config.JobQueue
	}

	batchClient, err := app.newBatchClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	jobID := opt.JobID
	if jobID == "" {
		// Find the latest job from queue using job definition name
		jobID, err = app.findLatestJobID(ctx, batchClient, opt.JobQueue)
		if err != nil {
			return err
		}
	}

	// Get job details to find log stream
	descOut, err := batchClient.DescribeJobs(ctx, &batch.DescribeJobsInput{
		Jobs: []string{jobID},
	})
	if err != nil {
		return fmt.Errorf("failed to describe job: %w", err)
	}
	if len(descOut.Jobs) == 0 {
		return fmt.Errorf("job %s not found", jobID)
	}

	job := descOut.Jobs[0]
	logGroup, logStream, err := extractLogInfo(job)
	if err != nil {
		return err
	}

	fmt.Printf("Job: %s (%s)\n", aws.ToString(job.JobName), aws.ToString(job.JobId))
	fmt.Printf("Log: %s / %s\n", logGroup, logStream)
	fmt.Println("---")

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(app.config.Region))
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}
	cwlClient := cloudwatchlogs.NewFromConfig(awsCfg)

	input := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(logGroup),
		LogStreamName: aws.String(logStream),
		StartFromHead: aws.Bool(true),
	}
	if opt.Since > 0 {
		startTime := time.Now().Add(-opt.Since).UnixMilli()
		input.StartTime = aws.Int64(startTime)
		input.StartFromHead = aws.Bool(false)
	}

	var prevToken string
	for {
		out, err := cwlClient.GetLogEvents(ctx, input)
		if err != nil {
			return fmt.Errorf("failed to get log events: %w", err)
		}

		for _, event := range out.Events {
			ts := time.UnixMilli(aws.ToInt64(event.Timestamp))
			fmt.Printf("%s  %s\n", ts.Format(time.RFC3339), aws.ToString(event.Message))
		}

		nextToken := aws.ToString(out.NextForwardToken)
		noNewEvents := nextToken == prevToken && len(out.Events) == 0

		if !opt.Follow {
			// In non-follow mode, paginate until no more events
			if noNewEvents {
				break
			}
			prevToken = nextToken
			input.NextToken = out.NextForwardToken
			input.StartTime = nil
			input.StartFromHead = nil
			continue
		}

		// In follow mode, wait for new events or job completion
		if noNewEvents {
			done, err := app.isJobDone(ctx, batchClient, jobID)
			if err != nil {
				return err
			}
			if done {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
		prevToken = nextToken
		input.NextToken = out.NextForwardToken
		input.StartTime = nil
		input.StartFromHead = nil
	}
	return nil
}

// findLatestJobID finds the most recent job for the configured job definition.
func (app *App) findLatestJobID(ctx context.Context, client *batch.Client, jobQueue string) (string, error) {
	if jobQueue == "" {
		return "", fmt.Errorf("job queue is required to find latest job: set job_queue in config or use --job-queue flag")
	}

	rendered, err := app.render(ctx)
	if err != nil {
		return "", err
	}
	converted := walkMap(rendered, toPascalCase)
	name, _ := converted.(map[string]any)["JobDefinitionName"].(string)
	if name == "" {
		return "", fmt.Errorf("jobDefinitionName is required in job definition")
	}

	// Search across all statuses to find the most recent job
	statuses := []batchTypes.JobStatus{
		batchTypes.JobStatusRunning,
		batchTypes.JobStatusSucceeded,
		batchTypes.JobStatusFailed,
		batchTypes.JobStatusStarting,
		batchTypes.JobStatusRunnable,
		batchTypes.JobStatusSubmitted,
		batchTypes.JobStatusPending,
	}

	type jobEntry struct {
		id        string
		createdAt int64
	}
	var candidates []jobEntry
	var lastErr error

	for _, status := range statuses {
		out, err := client.ListJobs(ctx, &batch.ListJobsInput{
			JobQueue:  aws.String(jobQueue),
			JobStatus: status,
			MaxResults: aws.Int32(5),
		})
		if err != nil {
			lastErr = err
			continue
		}
		for _, j := range out.JobSummaryList {
			if aws.ToString(j.JobName) == name || matchesJobDefinition(aws.ToString(j.JobDefinition), name) {
				candidates = append(candidates, jobEntry{
					id:        aws.ToString(j.JobId),
					createdAt: aws.ToInt64(j.CreatedAt),
				})
			}
		}
	}

	if len(candidates) == 0 {
		if lastErr != nil {
			return "", fmt.Errorf("failed to list jobs in queue %q: %w", jobQueue, lastErr)
		}
		return "", fmt.Errorf("no jobs found for %q in queue %q", name, jobQueue)
	}

	// Pick the most recently created job
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].createdAt > candidates[j].createdAt
	})
	return candidates[0].id, nil
}

// matchesJobDefinition checks if a job definition ARN matches the given name.
func matchesJobDefinition(arn, name string) bool {
	// ARN format: arn:aws:batch:region:account:job-definition/name:revision
	// Simple suffix match for the name portion
	for i := len(arn) - 1; i >= 0; i-- {
		if arn[i] == '/' {
			rest := arn[i+1:]
			// rest is "name:revision"
			for j := 0; j < len(rest); j++ {
				if rest[j] == ':' {
					return rest[:j] == name
				}
			}
			return rest == name
		}
	}
	return false
}

// extractLogInfo extracts CloudWatch log group and stream from a job.
func extractLogInfo(job batchTypes.JobDetail) (logGroup, logStream string, err error) {
	if job.Container != nil && job.Container.LogStreamName != nil {
		logStream = aws.ToString(job.Container.LogStreamName)
	}
	if logStream == "" {
		return "", "", fmt.Errorf("no log stream found for job %s (job may not have started yet, status: %s)",
			aws.ToString(job.JobId), job.Status)
	}

	// Determine log group from logConfiguration or use default
	logGroup = "/aws/batch/job"
	if job.Container != nil && job.Container.LogConfiguration != nil {
		if group, ok := job.Container.LogConfiguration.Options["awslogs-group"]; ok {
			logGroup = group
		}
	}
	return logGroup, logStream, nil
}

// isJobDone checks if the job has reached a terminal state.
func (app *App) isJobDone(ctx context.Context, client *batch.Client, jobID string) (bool, error) {
	out, err := client.DescribeJobs(ctx, &batch.DescribeJobsInput{
		Jobs: []string{jobID},
	})
	if err != nil {
		return false, fmt.Errorf("failed to describe job: %w", err)
	}
	if len(out.Jobs) == 0 {
		return true, nil
	}
	switch out.Jobs[0].Status {
	case batchTypes.JobStatusSucceeded, batchTypes.JobStatusFailed:
		return true, nil
	}
	return false, nil
}
