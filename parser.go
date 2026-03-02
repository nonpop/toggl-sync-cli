package main

import (
	"regexp"
	"strings"
)

var issueKeyRe = regexp.MustCompile(`^([A-Z][A-Z0-9]*-\d+)(?:\s+(.*))?$`)

func parseIssueKey(description string) (issueKey, remaining string, ok bool) {
	m := issueKeyRe.FindStringSubmatch(description)
	if m == nil {
		return "", "", false
	}
	return m[1], strings.TrimSpace(m[2]), true
}
