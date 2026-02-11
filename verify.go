package batcha

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/batch"
	batchTypes "github.com/aws/aws-sdk-go-v2/service/batch/types"
)

// Verify validates the job definition template locally without calling AWS.
func (app *App) Verify(ctx context.Context) error {
	rendered, err := app.render(ctx)
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}
	fmt.Println("OK: template rendered successfully")

	converted := walkMap(rendered, toPascalCase)
	jsonBytes, err := json.Marshal(converted)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	var input batch.RegisterJobDefinitionInput
	if err := json.Unmarshal(jsonBytes, &input); err != nil {
		return fmt.Errorf("unmarshal into RegisterJobDefinitionInput: %w", err)
	}
	fmt.Println("OK: valid RegisterJobDefinitionInput structure")

	errs := validateInput(&input)

	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Printf("NG: %s\n", e)
		}
		return fmt.Errorf("verification failed with %d error(s)", len(errs))
	}

	fmt.Println("OK: all validations passed")
	fmt.Println("Verify OK")
	return nil
}

func validateInput(input *batch.RegisterJobDefinitionInput) []string {
	var errs []string

	if input.JobDefinitionName == nil || *input.JobDefinitionName == "" {
		errs = append(errs, "jobDefinitionName is required")
	}

	if string(input.Type) == "" {
		errs = append(errs, "type is required")
	}

	isFargate := false
	for _, pc := range input.PlatformCapabilities {
		if pc == batchTypes.PlatformCapabilityFargate {
			isFargate = true
		}
	}

	switch string(input.Type) {
	case "container":
		errs = append(errs, validateContainerProperties(input.ContainerProperties, isFargate)...)
	case "multinode":
		if input.NodeProperties == nil {
			errs = append(errs, "nodeProperties is required when type is \"multinode\"")
		}
	}

	return errs
}

func validateContainerProperties(cp *batchTypes.ContainerProperties, isFargate bool) []string {
	var errs []string

	if cp == nil {
		return []string{"containerProperties is required when type is \"container\""}
	}

	if cp.Image == nil || *cp.Image == "" {
		errs = append(errs, "containerProperties.image is required")
	}

	if isFargate && (cp.ExecutionRoleArn == nil || *cp.ExecutionRoleArn == "") {
		errs = append(errs, "containerProperties.executionRoleArn is required for Fargate")
	}

	vcpu, memory := "", ""
	for _, r := range cp.ResourceRequirements {
		switch string(r.Type) {
		case "VCPU":
			vcpu = aws.ToString(r.Value)
		case "MEMORY":
			memory = aws.ToString(r.Value)
		}
	}

	if vcpu == "" {
		errs = append(errs, "containerProperties.resourceRequirements must include VCPU")
	} else if _, err := strconv.ParseFloat(vcpu, 64); err != nil {
		errs = append(errs, fmt.Sprintf("VCPU value %q is not a valid number", vcpu))
	}

	if memory == "" {
		errs = append(errs, "containerProperties.resourceRequirements must include MEMORY")
	} else if _, err := strconv.Atoi(memory); err != nil {
		errs = append(errs, fmt.Sprintf("MEMORY value %q is not a valid integer", memory))
	}

	if isFargate && vcpu != "" && memory != "" {
		errs = append(errs, validateFargateResources(vcpu, memory)...)
	}

	// Validate environment entries have non-empty names
	for i, env := range cp.Environment {
		if env.Name == nil || *env.Name == "" {
			errs = append(errs, fmt.Sprintf("containerProperties.environment[%d].name must not be empty", i))
		}
	}

	return errs
}

// fargateMemoryRanges defines allowed MEMORY values (in MiB) per VCPU.
// Ranges are [min, max, step].
var fargateMemoryRanges = map[string][3]int{
	"0.25": {512, 2048, 512},
	"0.5":  {1024, 4096, 1024},
	"1":    {2048, 8192, 1024},
	"2":    {4096, 16384, 1024},
	"4":    {8192, 30720, 1024},
	"8":    {16384, 61440, 4096},
	"16":   {32768, 122880, 8192},
}

func validateFargateResources(vcpu, memory string) []string {
	r, ok := fargateMemoryRanges[vcpu]
	if !ok {
		validVCPUs := "0.25, 0.5, 1, 2, 4, 8, 16"
		return []string{fmt.Sprintf("Fargate VCPU %q is not valid (allowed: %s)", vcpu, validVCPUs)}
	}

	mem, err := strconv.Atoi(memory)
	if err != nil {
		return nil // already caught by earlier validation
	}

	minMem, maxMem, step := r[0], r[1], r[2]
	if mem < minMem || mem > maxMem {
		return []string{fmt.Sprintf("Fargate MEMORY %d is out of range for VCPU %s (allowed: %d-%d MiB)", mem, vcpu, minMem, maxMem)}
	}
	if (mem-minMem)%step != 0 {
		return []string{fmt.Sprintf("Fargate MEMORY %d must be a multiple of %d (starting from %d) for VCPU %s", mem, step, minMem, vcpu)}
	}

	return nil
}
