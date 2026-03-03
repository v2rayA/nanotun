package exclude

import (
	"context"
	"log/slog"
	"net/netip"
	"strings"
	"sync"
	"time"

	gnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"

	M "github.com/xjasonlyu/tun2socks/v2/metadata"
)

// Matcher maps source flows to process names and decides whether a flow
// should bypass the tunnel entirely.
type Matcher struct {
	targets  map[string]struct{}
	interval time.Duration
	log      *slog.Logger

	mu        sync.RWMutex
	flowToPID map[string]int32
	pidNames  map[int32]string
}

// New creates a matcher. When no targets are provided the matcher is inert.
func New(targets []string, interval time.Duration, log *slog.Logger) *Matcher {
	if len(targets) == 0 {
		return nil
	}
	m := &Matcher{
		targets:   make(map[string]struct{}, len(targets)),
		interval:  interval,
		log:       log,
		flowToPID: make(map[string]int32),
		pidNames:  make(map[int32]string),
	}
	for _, name := range targets {
		m.targets[strings.ToLower(name)] = struct{}{}
	}
	if m.interval <= 0 {
		m.interval = 15 * time.Second
	}
	return m
}

// Start begins the periodic refresh loop.
func (m *Matcher) Start(ctx context.Context) {
	if m == nil {
		return
	}
	go m.refreshLoop(ctx)
}

// ShouldSkip reports whether the provided metadata belongs to an excluded process.
func (m *Matcher) ShouldSkip(md *M.Metadata) bool {
	if m == nil {
		return false
	}
	key := flowKey(md.Network, md.SourceAddrPort())
	if key == "" {
		return false
	}
	pid := m.lookupPID(key)
	if pid == 0 {
		return false
	}
	name := m.lookupName(pid)
	if name == "" {
		return false
	}
	_, skip := m.targets[strings.ToLower(name)]
	return skip
}

func (m *Matcher) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	m.scanOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.scanOnce(ctx)
		}
	}
}

func (m *Matcher) scanOnce(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	types := []string{"tcp", "tcp4", "tcp6", "udp", "udp4", "udp6"}
	nextFlow := make(map[string]int32)
	pidSet := make(map[int32]struct{})

	for _, kind := range types {
		conns, err := gnet.ConnectionsWithContext(ctx, kind)
		if err != nil {
			m.log.Debug("exclude: connection probe failed", "kind", kind, "err", err)
			continue
		}
		for _, c := range conns {
			if c.Status != "ESTABLISHED" && strings.HasPrefix(kind, "tcp") {
				continue
			}
			addr, err := netip.ParseAddr(c.Laddr.IP)
			if err != nil {
				continue
			}
			if c.Laddr.Port == 0 {
				continue
			}
			netProto := networkForKind(kind)
			key := flowKey(netProto, netip.AddrPortFrom(addr, uint16(c.Laddr.Port)))
			if key == "" {
				continue
			}
			pid := c.Pid
			nextFlow[key] = pid
			pidSet[pid] = struct{}{}
		}
	}

	names := m.resolveProcessNames(ctx, pidSet)

	m.mu.Lock()
	m.flowToPID = nextFlow
	m.pidNames = names // replace entirely to evict stale PIDs (prevents unbounded growth)
	m.mu.Unlock()
}

func (m *Matcher) resolveProcessNames(ctx context.Context, pids map[int32]struct{}) map[int32]string {
	names := make(map[int32]string, len(pids))
	for pid := range pids {
		if pid == 0 {
			continue
		}
		if cached := m.lookupName(pid); cached != "" {
			names[pid] = cached
			continue
		}
		proc, err := process.NewProcessWithContext(ctx, pid)
		if err != nil {
			continue
		}
		name, err := proc.NameWithContext(ctx)
		if err != nil {
			continue
		}
		names[pid] = strings.ToLower(name)
	}
	return names
}

func (m *Matcher) lookupPID(key string) int32 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.flowToPID[key]
}

func (m *Matcher) lookupName(pid int32) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pidNames[pid]
}

func flowKey(network M.Network, ap netip.AddrPort) string {
	if !ap.IsValid() {
		return ""
	}
	return network.String() + ":" + ap.String()
}

func networkForKind(kind string) M.Network {
	if strings.HasPrefix(kind, "udp") {
		return M.UDP
	}
	return M.TCP
}
