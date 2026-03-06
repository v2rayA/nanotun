package autoroute

import "testing"

func TestParseDefaultGatewayFromRoutePrint(t *testing.T) {
out := `===========================================================================
IPv4 Route Table
===========================================================================
Active Routes:
Network Destination        Netmask          Gateway       Interface  Metric
          0.0.0.0          0.0.0.0      192.168.31.1   192.168.31.7     35
===========================================================================`
got := parseDefaultGatewayFromRoutePrint(out)
if got != "192.168.31.1" {
t.Fatalf("gateway = %q, want %q", got, "192.168.31.1")
}
}

func TestParseDefaultGatewayFromRoutePrintNoMatch(t *testing.T) {
out := `Network Destination        Netmask          Gateway       Interface  Metric
       127.0.0.0        255.0.0.0         On-link        127.0.0.1    331`
if got := parseDefaultGatewayFromRoutePrint(out); got != "" {
t.Fatalf("gateway = %q, want empty", got)
}
}
