package diff

import (
	"fmt"
	"sort"
	"strings"
)

type Result struct {
	Added    []string
	Modified []string
	Removed  []string
}

func CompareManifests(from, to map[string]string) Result {
	result := Result{}
	for path, toHash := range to {
		fromHash, exists := from[path]
		if !exists {
			result.Added = append(result.Added, path)
			continue
		}
		if fromHash != toHash {
			result.Modified = append(result.Modified, path)
		}
	}
	for path := range from {
		if _, exists := to[path]; !exists {
			result.Removed = append(result.Removed, path)
		}
	}
	sort.Strings(result.Added)
	sort.Strings(result.Modified)
	sort.Strings(result.Removed)
	return result
}

// UnifiedDiff returns a compact line-by-line unified diff for MVP usage.
func UnifiedDiff(filename, oldContent, newContent string) string {
	if oldContent == newContent {
		return ""
	}
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	var out strings.Builder
	fmt.Fprintf(&out, "--- a/%s\n", filename)
	fmt.Fprintf(&out, "+++ b/%s\n", filename)

	maxLines := len(oldLines)
	if len(newLines) > maxLines {
		maxLines = len(newLines)
	}

	fmt.Fprintf(&out, "@@ -1,%d +1,%d @@\n", len(oldLines), len(newLines))
	for i := 0; i < maxLines; i++ {
		var oldLine string
		var newLine string
		haveOld := i < len(oldLines)
		haveNew := i < len(newLines)
		if haveOld {
			oldLine = oldLines[i]
		}
		if haveNew {
			newLine = newLines[i]
		}

		switch {
		case haveOld && haveNew && oldLine == newLine:
			fmt.Fprintf(&out, " %s\n", oldLine)
		case haveOld && haveNew:
			fmt.Fprintf(&out, "-%s\n", oldLine)
			fmt.Fprintf(&out, "+%s\n", newLine)
		case haveOld:
			fmt.Fprintf(&out, "-%s\n", oldLine)
		case haveNew:
			fmt.Fprintf(&out, "+%s\n", newLine)
		}
	}
	return out.String()
}

// ExpandedUnifiedDiff returns a unified diff with configurable context lines around changes.
func ExpandedUnifiedDiff(filename, oldContent, newContent string, contextLines int) string {
	if oldContent == newContent {
		return ""
	}
	if contextLines < 0 {
		contextLines = 0
	}

	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")
	changes := computeChanges(oldLines, newLines)
	if len(changes) == 0 {
		return ""
	}

	var out strings.Builder
	fmt.Fprintf(&out, "--- a/%s\n", filename)
	fmt.Fprintf(&out, "+++ b/%s\n", filename)

	for _, hunk := range groupHunks(changes, oldLines, newLines, contextLines) {
		out.WriteString(hunk)
	}
	return out.String()
}

type change struct {
	oldStart int
	oldEnd   int
	newStart int
	newEnd   int
}

func computeChanges(oldLines, newLines []string) []change {
	changes := make([]change, 0)
	i, j := 0, 0
	for i < len(oldLines) && j < len(newLines) {
		if oldLines[i] == newLines[j] {
			i++
			j++
			continue
		}

		oi, oj := i, j
		found := false
		for di := 0; di < 50 && oi+di < len(oldLines); di++ {
			for dj := 0; dj < 50 && oj+dj < len(newLines); dj++ {
				if oldLines[oi+di] == newLines[oj+dj] {
					changes = append(changes, change{
						oldStart: oi,
						oldEnd:   oi + di,
						newStart: oj,
						newEnd:   oj + dj,
					})
					i = oi + di
					j = oj + dj
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			changes = append(changes, change{
				oldStart: oi,
				oldEnd:   len(oldLines),
				newStart: oj,
				newEnd:   len(newLines),
			})
			return changes
		}
	}

	if i < len(oldLines) || j < len(newLines) {
		changes = append(changes, change{
			oldStart: i,
			oldEnd:   len(oldLines),
			newStart: j,
			newEnd:   len(newLines),
		})
	}
	return changes
}

func groupHunks(changes []change, oldLines, newLines []string, contextLines int) []string {
	hunks := make([]string, 0, len(changes))
	for _, c := range changes {
		oldFrom := max(0, c.oldStart-contextLines)
		oldTo := min(len(oldLines), c.oldEnd+contextLines)
		newFrom := max(0, c.newStart-contextLines)
		newTo := min(len(newLines), c.newEnd+contextLines)

		var hunk strings.Builder
		fmt.Fprintf(&hunk, "@@ -%d,%d +%d,%d @@\n", oldFrom+1, oldTo-oldFrom, newFrom+1, newTo-newFrom)

		for k := oldFrom; k < c.oldStart; k++ {
			fmt.Fprintf(&hunk, " %s\n", oldLines[k])
		}
		for k := c.oldStart; k < c.oldEnd; k++ {
			fmt.Fprintf(&hunk, "-%s\n", oldLines[k])
		}
		for k := c.newStart; k < c.newEnd; k++ {
			fmt.Fprintf(&hunk, "+%s\n", newLines[k])
		}
		for k := c.oldEnd; k < oldTo; k++ {
			fmt.Fprintf(&hunk, " %s\n", oldLines[k])
		}

		hunks = append(hunks, hunk.String())
	}
	return hunks
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
