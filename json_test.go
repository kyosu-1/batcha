package batcha

import "testing"

func TestToPascalCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"a", "A"},
		{"jobDefinitionName", "JobDefinitionName"},
		{"type", "Type"},
		{"containerProperties", "ContainerProperties"},
		{"VCPU", "VCPU"},
		{"Already", "Already"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toPascalCase(tt.input)
			if got != tt.want {
				t.Errorf("toPascalCase(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestToCamelCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"A", "a"},
		{"JobDefinitionName", "jobDefinitionName"},
		{"Type", "type"},
		{"ContainerProperties", "containerProperties"},
		{"VCPU", "vCPU"},
		{"already", "already"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toCamelCase(tt.input)
			if got != tt.want {
				t.Errorf("toCamelCase(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestWalkMap(t *testing.T) {
	input := map[string]any{
		"jobDefinitionName": "test-job",
		"containerProperties": map[string]any{
			"image": "nginx",
			"environment": []any{
				map[string]any{
					"name":  "FOO",
					"value": "bar",
				},
			},
		},
		"tags": map[string]any{
			"myTag": "value",
		},
		"parameters": map[string]any{
			"inputFile": "s3://bucket/file",
		},
	}

	result := walkMap(input, toPascalCase).(map[string]any)

	// Top-level keys should be PascalCase
	if _, ok := result["JobDefinitionName"]; !ok {
		t.Error("expected JobDefinitionName key")
	}

	// Nested keys should be PascalCase
	cp := result["ContainerProperties"].(map[string]any)
	if _, ok := cp["Image"]; !ok {
		t.Error("expected Image key in ContainerProperties")
	}

	// Environment inside array should be PascalCase
	envList := cp["Environment"].([]any)
	envItem := envList[0].(map[string]any)
	if _, ok := envItem["Name"]; !ok {
		t.Error("expected Name key in environment item")
	}

	// Tags keys should NOT be converted (skipConvertKeys)
	tags := result["Tags"].(map[string]any)
	if _, ok := tags["myTag"]; !ok {
		t.Error("expected tags keys to be preserved as-is")
	}

	// Parameters keys should NOT be converted (skipConvertKeys)
	params := result["Parameters"].(map[string]any)
	if _, ok := params["inputFile"]; !ok {
		t.Error("expected parameters keys to be preserved as-is")
	}
}

func TestWalkMap_ToCamelCase_SkipConvertKeys(t *testing.T) {
	// Simulate AWS response with PascalCase keys
	input := map[string]any{
		"JobDefinitionName": "test-job",
		"Tags": map[string]any{
			"ProjectName": "my-project",
			"Environment": "dev",
		},
		"Parameters": map[string]any{
			"InputFile": "s3://bucket/file",
		},
	}

	result := walkMap(input, toCamelCase).(map[string]any)

	// Top-level keys should be camelCase
	if _, ok := result["jobDefinitionName"]; !ok {
		t.Error("expected jobDefinitionName key")
	}

	// Tags children should NOT be converted even with PascalCase parent key
	tags := result["tags"].(map[string]any)
	if _, ok := tags["ProjectName"]; !ok {
		t.Error("expected Tags children to be preserved as-is, but ProjectName was converted")
	}

	// Parameters children should NOT be converted
	params := result["parameters"].(map[string]any)
	if _, ok := params["InputFile"]; !ok {
		t.Error("expected Parameters children to be preserved as-is, but InputFile was converted")
	}
}
