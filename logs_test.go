package batcha

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	batchTypes "github.com/aws/aws-sdk-go-v2/service/batch/types"
)

func TestMatchesJobDefinition(t *testing.T) {
	tests := []struct {
		arn  string
		name string
		want bool
	}{
		{
			arn:  "arn:aws:batch:us-east-1:123456789012:job-definition/my-job:3",
			name: "my-job",
			want: true,
		},
		{
			arn:  "arn:aws:batch:us-east-1:123456789012:job-definition/my-job:1",
			name: "other-job",
			want: false,
		},
		{
			arn:  "arn:aws:batch:ap-northeast-1:123456789012:job-definition/test-job:10",
			name: "test-job",
			want: true,
		},
		{
			arn:  "my-job",
			name: "my-job",
			want: false, // no slash, not an ARN
		},
	}
	for _, tt := range tests {
		t.Run(tt.arn+"_"+tt.name, func(t *testing.T) {
			got := matchesJobDefinition(tt.arn, tt.name)
			if got != tt.want {
				t.Errorf("matchesJobDefinition(%q, %q) = %v, want %v", tt.arn, tt.name, got, tt.want)
			}
		})
	}
}

func TestExtractLogInfo(t *testing.T) {
	t.Run("with_log_stream", func(t *testing.T) {
		job := batchTypes.JobDetail{
			JobId:   aws.String("job-123"),
			JobName: aws.String("my-job"),
			Status:  batchTypes.JobStatusSucceeded,
			Container: &batchTypes.ContainerDetail{
				LogStreamName: aws.String("my-job/default/abc123"),
			},
		}
		group, stream, err := extractLogInfo(job)
		if err != nil {
			t.Fatal(err)
		}
		if group != "/aws/batch/job" {
			t.Errorf("logGroup = %q, want /aws/batch/job", group)
		}
		if stream != "my-job/default/abc123" {
			t.Errorf("logStream = %q, want my-job/default/abc123", stream)
		}
	})

	t.Run("with_custom_log_group", func(t *testing.T) {
		job := batchTypes.JobDetail{
			JobId:   aws.String("job-456"),
			JobName: aws.String("my-job"),
			Status:  batchTypes.JobStatusSucceeded,
			Container: &batchTypes.ContainerDetail{
				LogStreamName: aws.String("my-job/default/def456"),
				LogConfiguration: &batchTypes.LogConfiguration{
					Options: map[string]string{
						"awslogs-group": "/custom/log/group",
					},
				},
			},
		}
		group, stream, err := extractLogInfo(job)
		if err != nil {
			t.Fatal(err)
		}
		if group != "/custom/log/group" {
			t.Errorf("logGroup = %q, want /custom/log/group", group)
		}
		if stream != "my-job/default/def456" {
			t.Errorf("logStream = %q, want my-job/default/def456", stream)
		}
	})

	t.Run("no_log_stream", func(t *testing.T) {
		job := batchTypes.JobDetail{
			JobId:   aws.String("job-789"),
			JobName: aws.String("my-job"),
			Status:  batchTypes.JobStatusPending,
		}
		_, _, err := extractLogInfo(job)
		if err == nil {
			t.Fatal("expected error for job with no log stream")
		}
	})

	t.Run("no_log_stream_with_empty_container", func(t *testing.T) {
		job := batchTypes.JobDetail{
			JobId:     aws.String("job-000"),
			JobName:   aws.String("my-job"),
			Status:    batchTypes.JobStatusSubmitted,
			Container: &batchTypes.ContainerDetail{},
		}
		_, _, err := extractLogInfo(job)
		if err == nil {
			t.Fatal("expected error for job with no log stream")
		}
	})
}
