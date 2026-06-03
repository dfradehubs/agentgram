package slack

import (
	"regexp"
	"strings"
)

// Formatter converts standard Markdown to Slack mrkdwn and handles message splitting.
type Formatter struct{}

var (
	reHeaders      = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
	reBold         = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reTableSep     = regexp.MustCompile(`(?m)^\|[-:\s|]+\|$`)
	reExcessNL     = regexp.MustCompile(`\n{3,}`)
	reCodeBlockLang = regexp.MustCompile("(?m)^```\\w*\\s*$")
)

// ToSlackMrkdwn converts Markdown to Slack mrkdwn format.
func (f *Formatter) ToSlackMrkdwn(md string) string {
	s := md
	// Headers → bold
	s = reHeaders.ReplaceAllString(s, "*$1*")
	// Bold: **text** → *text*
	s = reBold.ReplaceAllString(s, "*$1*")
	// Code block language hints → plain ```
	s = reCodeBlockLang.ReplaceAllString(s, "```")
	// Remove table separator rows
	s = reTableSep.ReplaceAllString(s, "")
	// Collapse excessive newlines
	s = reExcessNL.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

const maxSlackMessageLen = 30000

// Split breaks text into chunks that fit Slack's message size limit.
// Splits at paragraph boundaries, then sentence boundaries, then hard break.
func (f *Formatter) Split(text string, maxLen int) []string {
	if maxLen <= 0 {
		maxLen = maxSlackMessageLen
	}
	if len(text) <= maxLen {
		return []string{text}
	}

	var chunks []string
	remaining := text

	for len(remaining) > 0 {
		if len(remaining) <= maxLen {
			chunks = append(chunks, remaining)
			break
		}

		cutAt := findBreakPoint(remaining, maxLen)
		chunks = append(chunks, strings.TrimRight(remaining[:cutAt], "\n "))
		remaining = strings.TrimLeft(remaining[cutAt:], "\n ")
	}
	return chunks
}

func findBreakPoint(text string, maxLen int) int {
	// Try paragraph break
	if idx := strings.LastIndex(text[:maxLen], "\n\n"); idx > 0 {
		return idx + 2
	}
	// Try newline
	if idx := strings.LastIndex(text[:maxLen], "\n"); idx > 0 {
		return idx + 1
	}
	// Try sentence break
	for _, sep := range []string{". ", "! ", "? "} {
		if idx := strings.LastIndex(text[:maxLen], sep); idx > 0 {
			return idx + len(sep)
		}
	}
	// Try space
	if idx := strings.LastIndex(text[:maxLen], " "); idx > 0 {
		return idx + 1
	}
	// Hard break
	return maxLen
}
