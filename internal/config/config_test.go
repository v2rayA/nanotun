package config

import (
	"testing"

	"github.com/v2rayA/nanotun/internal/procname"
)

func TestNormalizeProcessListStripsExeAndDeduplicates(t *testing.T) {
	input := []string{"Shadowsocks.exe", "shadowsocks", "  Shadowsocks.EXE "}
	got := normalizeProcessList(input)

	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(got), got)
	}
	if got[0] != "shadowsocks" {
		t.Fatalf("expected normalized name 'shadowsocks', got %q", got[0])
	}
}

func TestNormalizeProcessNameTrimsWhitespace(t *testing.T) {
	if procname.Normalize("  Example.EXE ") != "example" {
		t.Fatalf("expected whitespace and suffix trimmed")
	}
}
