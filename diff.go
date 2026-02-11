package batcha

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/batch"
)

// Diff compares the local rendered definition with the active one on AWS.
// Returns an error wrapping DiffError if differences exist (exit code 1 for CI).
func (app *App) Diff(ctx context.Context) error {
	rendered, err := app.render(ctx)
	if err != nil {
		return err
	}
	converted := walkMap(rendered, toPascalCase)
	localBytes, err := json.MarshalIndent(converted, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal local definition: %w", err)
	}

	// Extract job definition name from the rendered template
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
		fmt.Printf("No active job definition found for %q. The local definition will be newly registered.\n", name)
		fmt.Println(string(localBytes))
		return &DiffError{}
	}

	// Pick the latest revision and strip AWS-managed fields
	latest := pickLatestRevision(out.JobDefinitions)

	remoteMap, err := normalizeRemoteDefinition(latest)
	if err != nil {
		return err
	}
	remoteBytes, err := json.MarshalIndent(remoteMap, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format remote definition: %w", err)
	}

	diff := unifiedDiff(string(remoteBytes), string(localBytes), "remote", "local")
	if diff == "" {
		fmt.Println("No differences found.")
		return nil
	}
	fmt.Println(diff)
	return &DiffError{}
}

// DiffError is returned when diff finds differences.
type DiffError struct{}

func (e *DiffError) Error() string { return "differences found" }

// --- Unified diff (stdlib only) ---

// unifiedDiff produces a unified diff string between two texts.
// Returns an empty string if there are no differences.
func unifiedDiff(a, b, labelA, labelB string) string {
	linesA := strings.Split(a, "\n")
	linesB := strings.Split(b, "\n")

	// Simple LCS-based diff
	lcs := lcsTable(linesA, linesB)
	hunks := buildHunks(linesA, linesB, lcs)
	if len(hunks) == 0 {
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n", labelA)
	fmt.Fprintf(&sb, "+++ %s\n", labelB)
	for _, h := range hunks {
		sb.WriteString(h)
	}
	return sb.String()
}

func lcsTable(a, b []string) [][]int {
	m, n := len(a), len(b)
	table := make([][]int, m+1)
	for i := range table {
		table[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if a[i] == b[j] {
				table[i][j] = table[i+1][j+1] + 1
			} else if table[i+1][j] >= table[i][j+1] {
				table[i][j] = table[i+1][j]
			} else {
				table[i][j] = table[i][j+1]
			}
		}
	}
	return table
}

type diffOp struct {
	kind byte // ' ', '-', '+'
	line string
	posA int
	posB int
}

func buildHunks(a, b []string, lcs [][]int) []string {
	ops := buildOps(a, b, lcs)
	if len(ops) == 0 {
		return nil
	}

	// Check if there are any actual changes
	hasChanges := false
	for _, op := range ops {
		if op.kind != ' ' {
			hasChanges = true
			break
		}
	}
	if !hasChanges {
		return nil
	}

	const ctx = 3
	var hunks []string
	var hunkOps []diffOp
	lastChange := -1

	for i, op := range ops {
		if op.kind != ' ' {
			if lastChange == -1 {
				// Start new hunk with context
				start := max(i-ctx, 0)
				hunkOps = ops[start:i]
			} else if i-lastChange > 2*ctx {
				// Flush previous hunk
				end := min(lastChange+ctx+1, len(ops))
				hunks = append(hunks, formatHunk(append(hunkOps, ops[lastChange+1:end]...)))
				// Start new hunk
				start := max(i-ctx, 0)
				hunkOps = make([]diffOp, 0)
				for j := start; j < i; j++ {
					hunkOps = append(hunkOps, ops[j])
				}
			} else {
				// Continue current hunk
				hunkOps = append(hunkOps, ops[lastChange+1:i]...)
			}
			hunkOps = append(hunkOps, op)
			lastChange = i
		}
	}

	// Flush final hunk
	if lastChange >= 0 {
		end := min(lastChange+ctx+1, len(ops))
		hunks = append(hunks, formatHunk(append(hunkOps, ops[lastChange+1:end]...)))
	}

	return hunks
}

func buildOps(a, b []string, lcs [][]int) []diffOp {
	var ops []diffOp
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i] == b[j] {
			ops = append(ops, diffOp{' ', a[i], i, j})
			i++
			j++
		} else if lcs[i+1][j] >= lcs[i][j+1] {
			ops = append(ops, diffOp{'-', a[i], i, j})
			i++
		} else {
			ops = append(ops, diffOp{'+', b[j], i, j})
			j++
		}
	}
	for ; i < len(a); i++ {
		ops = append(ops, diffOp{'-', a[i], i, j})
	}
	for ; j < len(b); j++ {
		ops = append(ops, diffOp{'+', b[j], i, j})
	}
	return ops
}

func formatHunk(ops []diffOp) string {
	if len(ops) == 0 {
		return ""
	}
	startA, startB := ops[0].posA+1, ops[0].posB+1
	countA, countB := 0, 0
	for _, op := range ops {
		switch op.kind {
		case ' ':
			countA++
			countB++
		case '-':
			countA++
		case '+':
			countB++
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", startA, countA, startB, countB)
	for _, op := range ops {
		fmt.Fprintf(&sb, "%c%s\n", op.kind, op.line)
	}
	return sb.String()
}
