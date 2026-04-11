package toonout

import (
	"encoding/json"
	"strconv"
	"strings"

	toon "github.com/toon-format/toon-go"
)

func Encode(v any) string {
	s, err := toon.MarshalString(v, toon.WithLengthMarkers(true))
	if err == nil {
		return strings.TrimRight(s, "\n")
	}

	fallback, jsonErr := json.MarshalIndent(v, "", "  ")
	if jsonErr != nil {
		return "{}"
	}
	return string(fallback)
}

func RenderHelp(lines []string) string {
	if len(lines) == 0 {
		return ""
	}

	indented := make([]string, 0, len(lines))
	for _, line := range lines {
		indented = append(indented, "  "+line)
	}

	return "help[" + itoa(len(lines)) + "]:\n" + strings.Join(indented, "\n")
}

func JoinBlocks(blocks ...string) string {
	filtered := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}
		filtered = append(filtered, block)
	}
	return strings.Join(filtered, "\n")
}

func itoa(v int) string {
	return strconv.Itoa(v)
}
