package snapshot

import (
	"fmt"
	"regexp"
	"strings"
)

type RefInfo struct {
	Ref   string
	Label string
	Type  string
}

type TruncationResult struct {
	Text        string
	Truncated   bool
	TotalLength int
}

var (
	legacyRefPattern     = regexp.MustCompile(`\buid=(\S+)\s+([\w-]+)\s+"([^"]*)"`)
	playwrightRefPattern = regexp.MustCompile(`^\s*-\s+([\w-]+)(?:\s+"([^"]*)")?.*?\[ref=([^]]+)\](?::\s*(.*))?`)
	refTokenPattern      = regexp.MustCompile(`\buid=([^\s]+)|\[ref=([^]]+)\]`)
	rootTitlePattern     = regexp.MustCompile(`RootWebArea\s+"([^"]+)"`)
	headingPattern       = regexp.MustCompile(`\bheading\s+"([^"]+)"`)
)

func CountRefs(text string) int {
	matches := refTokenPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return 0
	}
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		ref := ""
		if len(match) > 1 && match[1] != "" {
			ref = match[1]
		}
		if len(match) > 2 && match[2] != "" {
			ref = match[2]
		}
		if ref != "" {
			seen[ref] = struct{}{}
		}
	}
	return len(seen)
}

func ExtractRefs(text string) []RefInfo {
	lines := strings.Split(text, "\n")
	refs := make([]RefInfo, 0)
	for _, line := range lines {
		matches := legacyRefPattern.FindStringSubmatch(line)
		if len(matches) != 4 {
			if current := extractPlaywrightRef(line); current.Ref != "" {
				refs = append(refs, current)
			}
			continue
		}
		refs = append(refs, RefInfo{
			Ref:   matches[1],
			Type:  matches[2],
			Label: matches[3],
		})
	}
	return refs
}

func extractPlaywrightRef(line string) RefInfo {
	matches := playwrightRefPattern.FindStringSubmatch(line)
	if len(matches) != 5 {
		return RefInfo{}
	}
	label := strings.TrimSpace(matches[2])
	if label == "" {
		label = strings.TrimSpace(matches[4])
	}
	return RefInfo{
		Ref:   strings.TrimSpace(matches[3]),
		Type:  strings.TrimSpace(matches[1]),
		Label: label,
	}
}

func ExtractTitle(text string) string {
	if matches := rootTitlePattern.FindStringSubmatch(text); len(matches) == 2 {
		return matches[1]
	}
	if matches := headingPattern.FindStringSubmatch(text); len(matches) == 2 {
		return matches[1]
	}
	return ""
}

func StripSnapshotHeader(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "## Latest page snapshot\n", ""))
	if fenced := fencedYAML(text); fenced != "" {
		text = fenced
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.Contains(line, "RootWebArea") || strings.Contains(line, "uid=") || strings.Contains(line, "[ref=") {
			return strings.TrimSpace(strings.Join(lines[i:], "\n"))
		}
	}
	return strings.TrimSpace(text)
}

func fencedYAML(text string) string {
	const marker = "```yaml"
	idx := strings.Index(text, marker)
	if idx < 0 {
		return ""
	}
	after := strings.TrimLeft(text[idx+len(marker):], "\n")
	if end := strings.Index(after, "\n```"); end >= 0 {
		return strings.TrimSpace(after[:end])
	}
	return strings.TrimSpace(after)
}

func TruncateSnapshot(text string, full bool, limit int) TruncationResult {
	total := len(text)
	if full || total <= limit {
		return TruncationResult{Text: text, Truncated: false, TotalLength: total}
	}
	cut := strings.LastIndex(text[:limit], "\n")
	if cut <= 0 {
		cut = limit
	}
	return TruncationResult{
		Text:        text[:cut],
		Truncated:   true,
		TotalLength: total,
	}
}

func TruncateText(text string, limit int) TruncationResult {
	total := len(text)
	if total <= limit || total <= limit+50 {
		return TruncationResult{Text: text, Truncated: false, TotalLength: total}
	}
	headBudget := limit * 4 / 10
	tailBudget := limit - headBudget
	head := text[:headBudget]
	if cut := strings.LastIndex(head, "\n"); cut > 0 {
		head = head[:cut]
	}
	tail := text[total-tailBudget:]
	if cut := strings.Index(tail, "\n"); cut > 0 && cut+1 < len(tail) {
		tail = tail[cut+1:]
	}
	omitted := total - len(head) - len(tail)
	return TruncationResult{
		Text:        fmt.Sprintf("%s\n\n... (%d chars omitted, %d total) ...\n\n%s", head, omitted, total, tail),
		Truncated:   true,
		TotalLength: total,
	}
}

func IsInputType(kind string) bool {
	switch kind {
	case "textbox", "searchbox", "input", "combobox", "textarea":
		return true
	default:
		return false
	}
}
