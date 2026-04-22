package suggestions

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gvkhna/clawchrome-cli/internal/snapshot"
)

type Context struct {
	Command  string
	URL      string
	Snapshot string
}

var submitPattern = regexp.MustCompile(`(?i)submit|search|go|send|login|sign|ok`)

func Get(ctx Context) []string {
	if ctx.Command == "wait" {
		return []string{"Run `clawchrome-cli snapshot` to see current page state"}
	}

	refs := snapshot.ExtractRefs(ctx.Snapshot)
	buttons := make([]snapshot.RefInfo, 0)
	links := make([]snapshot.RefInfo, 0)
	inputs := make([]snapshot.RefInfo, 0)
	for _, ref := range refs {
		switch ref.Type {
		case "button":
			buttons = append(buttons, ref)
		case "link":
			links = append(links, ref)
		default:
			if snapshot.IsInputType(ref.Type) {
				inputs = append(inputs, ref)
			}
		}
	}

	lines := make([]string, 0)

	if ctx.Command == "fill" {
		for _, ref := range buttons {
			if submitPattern.MatchString(ref.Label) {
				lines = append(lines, fmt.Sprintf("Run `clawchrome-cli click @%s` to click %q", ref.Ref, ref.Label))
				break
			}
		}
		if len(lines) == 0 {
			lines = append(lines, "Run `clawchrome-cli press Enter` to submit the form")
		}
	}

	if len(inputs) > 0 && ctx.Command != "fill" {
		inp := inputs[0]
		label := "the input field"
		if inp.Label != "" {
			label = fmt.Sprintf("the %q field", inp.Label)
		}
		lines = append(lines, fmt.Sprintf("Run `clawchrome-cli fill @%s \"text\"` to fill %s", inp.Ref, label))
	}

	if len(buttons) > 0 {
		btn := buttons[0]
		if ctx.Command == "fill" {
			for _, candidate := range buttons {
				if !submitPattern.MatchString(candidate.Label) {
					btn = candidate
					break
				}
			}
		}
		if !hasRefSuggestion(lines, btn.Ref) {
			label := ""
			if btn.Label != "" {
				label = fmt.Sprintf("%q ", btn.Label)
			}
			lines = append(lines, fmt.Sprintf("Run `clawchrome-cli click @%s` to click the %sbutton", btn.Ref, label))
		}
	}

	if len(links) > 0 {
		link := links[0]
		lines = append(lines, fmt.Sprintf("Run `clawchrome-cli click @%s` to click the %q link", link.Ref, link.Label))
	}

	if len(buttons)+len(links)+len(inputs) > 5 {
		lines = append(lines, "Run `clawchrome-cli scroll down` to scroll down")
	}

	return lines
}

func hasRefSuggestion(lines []string, ref string) bool {
	for _, line := range lines {
		if strings.Contains(line, "@"+ref) {
			return true
		}
	}
	return false
}
