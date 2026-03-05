package autoroute

import (
	"strings"
	"testing"
)

func TestWithPowerShellUnicode(t *testing.T) {
	got := withPowerShellUnicode(" Get-NetRoute ")
	if !strings.Contains(got, "[System.Text.Encoding]::Unicode") {
		t.Fatalf("expected unicode encoding setup in script: %q", got)
	}
	if !strings.HasSuffix(got, "Get-NetRoute") {
		t.Fatalf("expected trimmed original script at end: %q", got)
	}
}
