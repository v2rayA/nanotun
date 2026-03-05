package autoroute

import "strings"

// parseDefaultGatewayFromRoutePrint extracts the gateway from route.exe output.
func parseDefaultGatewayFromRoutePrint(out string) string {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "0.0.0.0" && fields[1] == "0.0.0.0" {
			return fields[2]
		}
	}
	return ""
}
