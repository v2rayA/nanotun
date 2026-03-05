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

func TestBuildAddIPv4HostRouteScriptIncludesInterfaceIndexLookup(t *testing.T) {
	got := buildAddIPv4HostRouteScript("127.0.0.1", "192.168.12.1")
	if !strings.Contains(got, "Get-NetRoute -AddressFamily IPv4 -DestinationPrefix '0.0.0.0/0'") {
		t.Fatalf("expected default route lookup in script: %q", got)
	}
	if !strings.Contains(got, "-InterfaceIndex $ifIndex") {
		t.Fatalf("expected explicit interface index binding: %q", got)
	}
}

func TestBuildAddLocalhostRouteScriptIsIdempotent(t *testing.T) {
	got := buildAddLocalhostRouteScript("127.0.0.1", "127.0.0.0/8", "0.0.0.0")
	if !strings.Contains(got, "$existing = Get-NetRoute") {
		t.Fatalf("expected existing route check: %q", got)
	}
	if !strings.Contains(got, "if (-not $existing)") {
		t.Fatalf("expected conditional route creation: %q", got)
	}
}
