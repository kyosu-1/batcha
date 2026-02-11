package batcha

import (
	"strings"
	"testing"
)

func TestUnifiedDiff_NoDiff(t *testing.T) {
	a := "line1\nline2\nline3"
	b := "line1\nline2\nline3"
	diff := unifiedDiff(a, b, "a", "b")
	if diff != "" {
		t.Errorf("expected empty diff, got:\n%s", diff)
	}
}

func TestUnifiedDiff_WithChanges(t *testing.T) {
	a := "line1\nline2\nline3"
	b := "line1\nmodified\nline3"
	diff := unifiedDiff(a, b, "a", "b")
	if diff == "" {
		t.Error("expected non-empty diff")
	}
	// Should contain unified diff markers
	if !strings.Contains(diff, "---") || !strings.Contains(diff, "+++") || !strings.Contains(diff, "@@") {
		t.Errorf("diff missing markers:\n%s", diff)
	}
	if !strings.Contains(diff, "-line2") || !strings.Contains(diff, "+modified") {
		t.Errorf("diff missing expected lines:\n%s", diff)
	}
}
