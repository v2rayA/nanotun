package autoroute

import "strings"

func withPowerShellUnicode(script string) string {
	return "$OutputEncoding = [Console]::OutputEncoding = [System.Text.Encoding]::Unicode; " + strings.TrimSpace(script)
}
