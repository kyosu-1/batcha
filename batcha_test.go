package batcha

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	batchTypes "github.com/aws/aws-sdk-go-v2/service/batch/types"
)

func TestNormalizeRemoteDefinition(t *testing.T) {
	def := batchTypes.JobDefinition{
		JobDefinitionArn:             aws.String("arn:aws:batch:ap-northeast-1:123456789012:job-definition/test-job:1"),
		JobDefinitionName:            aws.String("test-job"),
		Revision:                     aws.Int32(1),
		Status:                       aws.String("ACTIVE"),
		ContainerOrchestrationType:   "ECS",
		Type:                         aws.String("container"),
		PlatformCapabilities:         []batchTypes.PlatformCapability{batchTypes.PlatformCapabilityFargate},
		ContainerProperties: &batchTypes.ContainerProperties{
			Image:   aws.String("nginx:latest"),
			Command: []string{"/bin/sh"},
			ResourceRequirements: []batchTypes.ResourceRequirement{
				{Type: batchTypes.ResourceTypeVcpu, Value: aws.String("0.25")},
				{Type: batchTypes.ResourceTypeMemory, Value: aws.String("512")},
			},
		},
	}

	m, err := normalizeRemoteDefinition(def)
	if err != nil {
		t.Fatalf("normalizeRemoteDefinition failed: %v", err)
	}

	// AWS-managed fields should be stripped
	for _, key := range initExcludeKeys {
		if _, ok := m[key]; ok {
			t.Errorf("expected key %q to be removed, but it exists", key)
		}
	}

	// User-managed fields should remain
	if _, ok := m["JobDefinitionName"]; !ok {
		t.Error("expected JobDefinitionName to remain")
	}
	if _, ok := m["Type"]; !ok {
		t.Error("expected Type to remain")
	}
	if _, ok := m["ContainerProperties"]; !ok {
		t.Error("expected ContainerProperties to remain")
	}
}

func TestPickLatestRevision(t *testing.T) {
	defs := []batchTypes.JobDefinition{
		{JobDefinitionName: aws.String("job"), Revision: aws.Int32(1)},
		{JobDefinitionName: aws.String("job"), Revision: aws.Int32(3)},
		{JobDefinitionName: aws.String("job"), Revision: aws.Int32(2)},
	}
	latest := pickLatestRevision(defs)
	if aws.ToInt32(latest.Revision) != 3 {
		t.Errorf("expected revision 3, got %d", aws.ToInt32(latest.Revision))
	}
}
