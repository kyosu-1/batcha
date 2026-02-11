package batcha

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/batch"
	batchTypes "github.com/aws/aws-sdk-go-v2/service/batch/types"
)

func TestVerify_OK(t *testing.T) {
	t.Setenv("TEST_JOB_NAME", "verify-job")

	app, err := New(context.Background(), filepath.Join("testdata", "config.yml"))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if err := app.Verify(context.Background()); err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
}

func TestVerify_MissingName(t *testing.T) {
	app := verifyApp(t, `{
  "type": "container",
  "containerProperties": {
    "image": "nginx",
    "resourceRequirements": [
      {"type": "VCPU", "value": "1"},
      {"type": "MEMORY", "value": "2048"}
    ]
  }
}`)
	err := app.Verify(context.Background())
	if err == nil {
		t.Fatal("expected error for missing jobDefinitionName")
	}
}

func TestVerify_MissingContainerProperties(t *testing.T) {
	app := verifyApp(t, `{
  "jobDefinitionName": "test",
  "type": "container"
}`)
	err := app.Verify(context.Background())
	if err == nil {
		t.Fatal("expected error for missing containerProperties")
	}
}

func TestVerify_MissingResourceRequirements(t *testing.T) {
	app := verifyApp(t, `{
  "jobDefinitionName": "test",
  "type": "container",
  "containerProperties": {
    "image": "nginx"
  }
}`)
	err := app.Verify(context.Background())
	if err == nil {
		t.Fatal("expected error for missing resource requirements")
	}
}

// --- validateInput unit tests ---

func TestValidateInput_Fargate_MissingExecutionRole(t *testing.T) {
	input := &batch.RegisterJobDefinitionInput{
		JobDefinitionName:    aws.String("test"),
		Type:                 batchTypes.JobDefinitionTypeContainer,
		PlatformCapabilities: []batchTypes.PlatformCapability{batchTypes.PlatformCapabilityFargate},
		ContainerProperties: &batchTypes.ContainerProperties{
			Image: aws.String("nginx"),
			ResourceRequirements: []batchTypes.ResourceRequirement{
				{Type: batchTypes.ResourceTypeVcpu, Value: aws.String("0.25")},
				{Type: batchTypes.ResourceTypeMemory, Value: aws.String("512")},
			},
		},
	}
	errs := validateInput(input)
	if !containsSubstring(errs, "executionRoleArn is required for Fargate") {
		t.Errorf("expected executionRoleArn error, got: %v", errs)
	}
}

func TestValidateInput_Fargate_InvalidVCPU(t *testing.T) {
	input := &batch.RegisterJobDefinitionInput{
		JobDefinitionName:    aws.String("test"),
		Type:                 batchTypes.JobDefinitionTypeContainer,
		PlatformCapabilities: []batchTypes.PlatformCapability{batchTypes.PlatformCapabilityFargate},
		ContainerProperties: &batchTypes.ContainerProperties{
			Image:           aws.String("nginx"),
			ExecutionRoleArn: aws.String("arn:aws:iam::123456789012:role/test"),
			ResourceRequirements: []batchTypes.ResourceRequirement{
				{Type: batchTypes.ResourceTypeVcpu, Value: aws.String("3")},
				{Type: batchTypes.ResourceTypeMemory, Value: aws.String("8192")},
			},
		},
	}
	errs := validateInput(input)
	if !containsSubstring(errs, "Fargate VCPU") {
		t.Errorf("expected Fargate VCPU error, got: %v", errs)
	}
}

func TestValidateInput_Fargate_MemoryOutOfRange(t *testing.T) {
	input := &batch.RegisterJobDefinitionInput{
		JobDefinitionName:    aws.String("test"),
		Type:                 batchTypes.JobDefinitionTypeContainer,
		PlatformCapabilities: []batchTypes.PlatformCapability{batchTypes.PlatformCapabilityFargate},
		ContainerProperties: &batchTypes.ContainerProperties{
			Image:           aws.String("nginx"),
			ExecutionRoleArn: aws.String("arn:aws:iam::123456789012:role/test"),
			ResourceRequirements: []batchTypes.ResourceRequirement{
				{Type: batchTypes.ResourceTypeVcpu, Value: aws.String("0.25")},
				{Type: batchTypes.ResourceTypeMemory, Value: aws.String("4096")},
			},
		},
	}
	errs := validateInput(input)
	if !containsSubstring(errs, "out of range") {
		t.Errorf("expected memory out of range error, got: %v", errs)
	}
}

func TestValidateInput_Fargate_MemoryBadStep(t *testing.T) {
	input := &batch.RegisterJobDefinitionInput{
		JobDefinitionName:    aws.String("test"),
		Type:                 batchTypes.JobDefinitionTypeContainer,
		PlatformCapabilities: []batchTypes.PlatformCapability{batchTypes.PlatformCapabilityFargate},
		ContainerProperties: &batchTypes.ContainerProperties{
			Image:           aws.String("nginx"),
			ExecutionRoleArn: aws.String("arn:aws:iam::123456789012:role/test"),
			ResourceRequirements: []batchTypes.ResourceRequirement{
				{Type: batchTypes.ResourceTypeVcpu, Value: aws.String("8")},
				{Type: batchTypes.ResourceTypeMemory, Value: aws.String("20000")},
			},
		},
	}
	errs := validateInput(input)
	if !containsSubstring(errs, "must be a multiple") {
		t.Errorf("expected memory step error, got: %v", errs)
	}
}

func TestValidateInput_Fargate_ValidCombo(t *testing.T) {
	input := &batch.RegisterJobDefinitionInput{
		JobDefinitionName:    aws.String("test"),
		Type:                 batchTypes.JobDefinitionTypeContainer,
		PlatformCapabilities: []batchTypes.PlatformCapability{batchTypes.PlatformCapabilityFargate},
		ContainerProperties: &batchTypes.ContainerProperties{
			Image:           aws.String("nginx"),
			ExecutionRoleArn: aws.String("arn:aws:iam::123456789012:role/test"),
			ResourceRequirements: []batchTypes.ResourceRequirement{
				{Type: batchTypes.ResourceTypeVcpu, Value: aws.String("1")},
				{Type: batchTypes.ResourceTypeMemory, Value: aws.String("4096")},
			},
		},
	}
	errs := validateInput(input)
	if len(errs) > 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

func TestValidateInput_EC2_SkipFargateCheck(t *testing.T) {
	input := &batch.RegisterJobDefinitionInput{
		JobDefinitionName: aws.String("test"),
		Type:              batchTypes.JobDefinitionTypeContainer,
		ContainerProperties: &batchTypes.ContainerProperties{
			Image: aws.String("nginx"),
			ResourceRequirements: []batchTypes.ResourceRequirement{
				{Type: batchTypes.ResourceTypeVcpu, Value: aws.String("3")},
				{Type: batchTypes.ResourceTypeMemory, Value: aws.String("7777")},
			},
		},
	}
	errs := validateInput(input)
	if len(errs) > 0 {
		t.Errorf("EC2 should not trigger Fargate validation, got: %v", errs)
	}
}

func TestValidateInput_InvalidResourceValues(t *testing.T) {
	input := &batch.RegisterJobDefinitionInput{
		JobDefinitionName: aws.String("test"),
		Type:              batchTypes.JobDefinitionTypeContainer,
		ContainerProperties: &batchTypes.ContainerProperties{
			Image: aws.String("nginx"),
			ResourceRequirements: []batchTypes.ResourceRequirement{
				{Type: batchTypes.ResourceTypeVcpu, Value: aws.String("abc")},
				{Type: batchTypes.ResourceTypeMemory, Value: aws.String("xyz")},
			},
		},
	}
	errs := validateInput(input)
	if !containsSubstring(errs, "VCPU value") || !containsSubstring(errs, "MEMORY value") {
		t.Errorf("expected invalid value errors, got: %v", errs)
	}
}

func TestValidateInput_Multinode_MissingNodeProperties(t *testing.T) {
	input := &batch.RegisterJobDefinitionInput{
		JobDefinitionName: aws.String("test"),
		Type:              batchTypes.JobDefinitionTypeMultinode,
	}
	errs := validateInput(input)
	if !containsSubstring(errs, "nodeProperties is required") {
		t.Errorf("expected nodeProperties error, got: %v", errs)
	}
}

// --- Fargate memory range table tests ---

func TestFargateMemoryRanges(t *testing.T) {
	tests := []struct {
		vcpu   string
		memory string
		ok     bool
	}{
		{"0.25", "512", true},
		{"0.25", "1024", true},
		{"0.25", "2048", true},
		{"0.25", "256", false},
		{"0.25", "3072", false},
		{"0.5", "1024", true},
		{"0.5", "4096", true},
		{"0.5", "512", false},
		{"1", "2048", true},
		{"1", "8192", true},
		{"1", "1024", false},
		{"2", "4096", true},
		{"2", "16384", true},
		{"2", "3072", false},
		{"4", "8192", true},
		{"4", "30720", true},
		{"4", "7168", false},
		{"8", "16384", true},
		{"8", "61440", true},
		{"8", "20000", false}, // not aligned to 4096 step
		{"16", "32768", true},
		{"16", "122880", true},
		{"16", "40000", false}, // not aligned to 8192 step
	}
	for _, tt := range tests {
		t.Run(tt.vcpu+"vcpu_"+tt.memory+"mb", func(t *testing.T) {
			errs := validateFargateResources(tt.vcpu, tt.memory)
			if tt.ok && len(errs) > 0 {
				t.Errorf("expected valid, got errors: %v", errs)
			}
			if !tt.ok && len(errs) == 0 {
				t.Error("expected error for invalid combination")
			}
		})
	}
}

// helpers

func verifyApp(t *testing.T, jobDefJSON string) *App {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "job.json"), []byte(jobDefJSON), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "batcha.yml"), []byte("region: us-east-1\njob_definition: job.json\n"), 0644); err != nil {
		t.Fatal(err)
	}
	app, err := New(context.Background(), filepath.Join(dir, "batcha.yml"))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	return app
}

func containsSubstring(ss []string, sub string) bool {
	for _, s := range ss {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
