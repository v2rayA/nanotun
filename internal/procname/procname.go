package procname

import "strings"

// Normalize lowercases, trims whitespace, and strips a trailing ".exe" suffix
// for cross-platform process name matching (e.g., Windows executables).
func Normalize(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if strings.HasSuffix(n, ".exe") {
		n = strings.TrimSuffix(n, ".exe")
	}
	return n
}
