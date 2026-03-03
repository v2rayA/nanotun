package config

import "testing"

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
	if normalizeProcessName("  Example.EXE ") != "example" {
		t.Fatalf("expected whitespace and suffix trimmed")
	}
}
