package main

import "testing"

func TestParseIssueKey(t *testing.T) {
	tests := []struct {
		description string
		wantKey     string
		wantDesc    string
		wantOK      bool
	}{
		{"PROJ-123 fixed the bug", "PROJ-123", "fixed the bug", true},
		{"ABC-1 review", "ABC-1", "review", true},
		{"DATA2-99 migration script", "DATA2-99", "migration script", true},
		{"PROJ-123", "PROJ-123", "", true},
		{"no issue key here", "", "", false},
		{"proj-123 lowercase", "", "", false},
		{"123-ABC wrong format", "", "", false},
		{"", "", "", false},
		{"A-1 single letter project", "A-1", "single letter project", true},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			key, desc, ok := parseIssueKey(tt.description)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if key != tt.wantKey {
				t.Errorf("key = %q, want %q", key, tt.wantKey)
			}
			if desc != tt.wantDesc {
				t.Errorf("desc = %q, want %q", desc, tt.wantDesc)
			}
		})
	}
}
