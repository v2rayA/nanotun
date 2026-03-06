package autoroute

import "strings"

func withPowerShellUnicode(script string) string {
	return "$OutputEncoding = [Console]::OutputEncoding = [System.Text.Encoding]::Unicode; " + strings.TrimSpace(script)
}

func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func buildAddIPv4HostRouteScript(ip, nextHop string) string {
	return "$ifIndex = (Get-NetRoute -AddressFamily IPv4 -DestinationPrefix '0.0.0.0/0' -NextHop " + psQuote(nextHop) +
		" -ErrorAction SilentlyContinue | Sort-Object RouteMetric | Select-Object -First 1).InterfaceIndex; " +
		"if (-not $ifIndex) { throw 'failed to resolve interface index for default route' }; " +
		"New-NetRoute -AddressFamily IPv4 " +
		"-DestinationPrefix " + psQuote(ip+"/32") + " " +
		"-InterfaceIndex $ifIndex " +
		"-NextHop " + psQuote(nextHop) + " " +
		"-RouteMetric 1 -PolicyStore ActiveStore -ErrorAction Stop"
}

func buildAddLocalhostRouteScript(loopbackAddr, loopbackPrefix, nextHopOnLink string) string {
	return "$ifIndex = (Get-NetIPAddress -AddressFamily IPv4 -IPAddress " + psQuote(loopbackAddr) + " -ErrorAction Stop | " +
		"Select-Object -First 1).InterfaceIndex; " +
		"$existing = Get-NetRoute -AddressFamily IPv4 -DestinationPrefix " + psQuote(loopbackPrefix) + " " +
		"-InterfaceIndex $ifIndex -NextHop " + psQuote(nextHopOnLink) + " -ErrorAction SilentlyContinue | Select-Object -First 1; " +
		"if (-not $existing) { " +
		"New-NetRoute -AddressFamily IPv4 -DestinationPrefix " + psQuote(loopbackPrefix) + " " +
		"-InterfaceIndex $ifIndex -NextHop " + psQuote(nextHopOnLink) + " -RouteMetric 1 -PolicyStore ActiveStore -ErrorAction Stop }"
}
