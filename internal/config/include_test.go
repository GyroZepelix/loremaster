package config

import (
	"strings"
	"testing"
)

func TestParseIncludeEntry(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantSrc string
		wantDst string
		wantErr string
	}{
		// AC 1: backward compat identity mapping
		{
			name:    "simple name identity mapping",
			raw:     "brainstorm",
			wantSrc: "brainstorm",
			wantDst: "brainstorm",
		},
		// AC 2: nested identity mapping
		{
			name:    "nested path identity mapping",
			raw:     "loa/brainstorm",
			wantSrc: "loa/brainstorm",
			wantDst: "loa/brainstorm",
		},
		// AC 3: src:dst remap
		{
			name:    "src:dst remap",
			raw:     "deep/skill:my-tool",
			wantSrc: "deep/skill",
			wantDst: "my-tool",
		},
		// AC 4: empty destination
		{
			name:    "empty destination after colon",
			raw:     "loa/brainstorm:",
			wantErr: "must not be empty",
		},
		// AC 5: empty source
		{
			name:    "empty source before colon",
			raw:     ":brainstorm",
			wantErr: "must not be empty",
		},
		// AC 6: multiple colons — split on first only, dst has colon → error
		{
			name:    "multiple colons splits on first",
			raw:     "a:b:c",
			wantErr: "colons are not allowed",
		},
		// AC 7: path traversal on source side
		{
			name:    "path traversal source",
			raw:     "../escape:foo",
			wantErr: "must not escape root",
		},
		// AC 8: path traversal on destination side
		{
			name:    "path traversal destination",
			raw:     "foo:../escape",
			wantErr: "must not escape root",
		},
		// AC 9: absolute path
		{
			name:    "absolute path",
			raw:     "/absolute/path",
			wantErr: "must be relative",
		},
		// AC 10: empty string
		{
			name:    "empty string",
			raw:     "",
			wantErr: "must not be empty",
		},
		// AC 14: paths are cleaned
		{
			name:    "path with trailing slash is cleaned",
			raw:     "foo/bar/",
			wantSrc: "foo/bar",
			wantDst: "foo/bar",
		},
		{
			name:    "path with dot components is cleaned",
			raw:     "foo/./bar",
			wantSrc: "foo/bar",
			wantDst: "foo/bar",
		},
		{
			name:    "path with double slashes is cleaned",
			raw:     "foo//bar",
			wantSrc: "foo/bar",
			wantDst: "foo/bar",
		},
		// AC 15: backslashes rejected
		{
			name:    "backslash in path rejected",
			raw:     `foo\bar`,
			wantErr: "backslashes are not allowed",
		},
		// Control characters rejected
		{
			name:    "null byte in path rejected",
			raw:     "foo\x00bar",
			wantErr: "control characters are not allowed",
		},
		{
			name:    "newline in path rejected",
			raw:     "foo\nbar",
			wantErr: "control characters are not allowed",
		},
		{
			name:    "tab in path rejected",
			raw:     "foo\tbar",
			wantErr: "control characters are not allowed",
		},
		// Edge cases
		{
			name:    "traversal nested in path",
			raw:     "foo/../../etc",
			wantErr: "must not escape root",
		},
		{
			name:    "dot-dot only source",
			raw:     "..:foo",
			wantErr: "must not escape root",
		},
		{
			name:    "single dot resolves to empty",
			raw:     ".",
			wantErr: "must not be empty",
		},
		{
			name:    "remap with nested src and dst",
			raw:     "a/b/c:x/y",
			wantSrc: "a/b/c",
			wantDst: "x/y",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseIncludeEntry(tt.raw)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want containing %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Src != tt.wantSrc {
				t.Errorf("Src = %q, want %q", got.Src, tt.wantSrc)
			}
			if got.Dst != tt.wantDst {
				t.Errorf("Dst = %q, want %q", got.Dst, tt.wantDst)
			}
		})
	}
}

func TestValidateOverlaps(t *testing.T) {
	tests := []struct {
		name    string
		entries []IncludeEntry
		wantErr string
	}{
		// AC 11: prefix conflict
		{
			name: "prefix conflict parent and child",
			entries: []IncludeEntry{
				{Src: "loa", Dst: "loa"},
				{Src: "loa/brainstorm", Dst: "loa/brainstorm"},
			},
			wantErr: "overlapping include destinations",
		},
		// AC 12: no overlap
		{
			name: "no overlap distinct paths",
			entries: []IncludeEntry{
				{Src: "skill-a", Dst: "skill-a"},
				{Src: "skill-b", Dst: "skill-b"},
			},
		},
		// AC 13: duplicate destination
		{
			name: "duplicate destination",
			entries: []IncludeEntry{
				{Src: "a/b", Dst: "a/b"},
				{Src: "c/d", Dst: "a/b"},
			},
			wantErr: "duplicate include destination",
		},
		// No false positive on prefix-like names
		{
			name: "no false positive on similar prefix names",
			entries: []IncludeEntry{
				{Src: "loa", Dst: "loa"},
				{Src: "loa-extra", Dst: "loa-extra"},
			},
		},
		// Empty list is fine
		{
			name:    "empty entries list",
			entries: []IncludeEntry{},
		},
		// Single entry is fine
		{
			name: "single entry no conflict",
			entries: []IncludeEntry{
				{Src: "foo", Dst: "foo"},
			},
		},
		// Reverse order prefix conflict
		{
			name: "child before parent still detects overlap",
			entries: []IncludeEntry{
				{Src: "a/b/c", Dst: "x/y/z"},
				{Src: "d", Dst: "x/y"},
			},
			wantErr: "overlapping include destinations",
		},
		// Three entries with conflict in last pair
		{
			name: "three entries conflict in last pair",
			entries: []IncludeEntry{
				{Src: "a", Dst: "a"},
				{Src: "b", Dst: "b"},
				{Src: "c", Dst: "b"},
			},
			wantErr: "duplicate include destination",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOverlaps(tt.entries)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want containing %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
