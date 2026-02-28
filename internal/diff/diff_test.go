package diff

import "testing"

func TestCompareManifests(t *testing.T) {
	from := map[string]string{
		"a.go": "1",
		"b.go": "2",
		"c.go": "3",
	}
	to := map[string]string{
		"a.go": "1",
		"b.go": "22",
		"d.go": "4",
	}
	result := CompareManifests(from, to)
	if len(result.Added) != 1 || result.Added[0] != "d.go" {
		t.Fatalf("unexpected added: %+v", result.Added)
	}
	if len(result.Modified) != 1 || result.Modified[0] != "b.go" {
		t.Fatalf("unexpected modified: %+v", result.Modified)
	}
	if len(result.Removed) != 1 || result.Removed[0] != "c.go" {
		t.Fatalf("unexpected removed: %+v", result.Removed)
	}
}

func TestUnifiedDiff(t *testing.T) {
	out := UnifiedDiff("main.go", "a\nb\n", "a\nc\n")
	if out == "" {
		t.Fatalf("expected non-empty diff")
	}
	if !contains(out, "--- a/main.go") || !contains(out, "+++ b/main.go") {
		t.Fatalf("unexpected diff header: %s", out)
	}
}

func TestExpandedUnifiedDiff(t *testing.T) {
	oldContent := "a\nb\nc\nd\n"
	newContent := "a\nb\nx\nd\n"
	out := ExpandedUnifiedDiff("main.go", oldContent, newContent, 1)
	if out == "" {
		t.Fatalf("expected non-empty expanded diff")
	}
	if !contains(out, "@@ -2,3 +2,3 @@") {
		t.Fatalf("expected hunk header with context, got: %s", out)
	}
	if !contains(out, "-c") || !contains(out, "+x") {
		t.Fatalf("expected changed lines in expanded diff: %s", out)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && index(s, sub) >= 0
}

func index(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
