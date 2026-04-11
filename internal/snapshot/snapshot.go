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
	refPattern       = regexp.MustCompile(`\buid=(\S+)\s+(\w+)\s+"([^"]*)"`)
	rootTitlePattern = regexp.MustCompile(`RootWebArea\s+"([^"]+)"`)
	headingPattern   = regexp.MustCompile(`\bheading\s+"([^"]+)"`)
)

func CountRefs(text string) int {
	return strings.Count(text, "uid=")
}

func ExtractRefs(text string) []RefInfo {
	lines := strings.Split(text, "\n")
	refs := make([]RefInfo, 0)
	for _, line := range lines {
		matches := refPattern.FindStringSubmatch(line)
		if len(matches) != 4 {
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
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.Contains(line, "RootWebArea") || strings.Contains(line, "uid=") {
			return strings.Join(lines[i:], "\n")
		}
	}
	return strings.TrimSpace(strings.ReplaceAll(text, "## Latest page snapshot\n", ""))
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
