package metrics

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	devopsv1 "silent-devops/api/devops/v1"
)

type Inventory struct{ Hostname, OS, Kernel, Architecture, BootID string }
type Collector struct {
	Root          string
	MaxInterfaces int
	Now           func() time.Time
}

func (c Collector) Collect(ctx context.Context) (*devopsv1.MetricsSnapshot, Inventory, error) {
	if err := ctx.Err(); err != nil {
		return nil, Inventory{}, err
	}
	root := c.Root
	if root == "" {
		root = "/"
	}
	read := func(name string) ([]byte, error) { return os.ReadFile(filepath.Join(root, name)) }
	stat, err := read("proc/stat")
	if err != nil {
		return nil, Inventory{}, err
	}
	cpu, err := ParseCPU(stat)
	if err != nil {
		return nil, Inventory{}, err
	}
	memRaw, err := read("proc/meminfo")
	if err != nil {
		return nil, Inventory{}, err
	}
	mem, err := ParseMeminfo(memRaw)
	if err != nil {
		return nil, Inventory{}, err
	}
	loadRaw, err := read("proc/loadavg")
	if err != nil {
		return nil, Inventory{}, err
	}
	load, err := ParseLoad(loadRaw)
	if err != nil {
		return nil, Inventory{}, err
	}
	uptimeRaw, err := read("proc/uptime")
	if err != nil {
		return nil, Inventory{}, err
	}
	uptime, err := ParseUptime(uptimeRaw)
	if err != nil {
		return nil, Inventory{}, err
	}
	networkRaw, err := read("proc/net/dev")
	if err != nil {
		return nil, Inventory{}, err
	}
	network, err := ParseNetwork(networkRaw, c.MaxInterfaces)
	if err != nil {
		return nil, Inventory{}, err
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	metrics := []*devopsv1.Metric{{Name: "cpu_total_ticks", Value: float64(cpu.Total)}, {Name: "cpu_idle_ticks", Value: float64(cpu.Idle)}, {Name: "memory_total_bytes", Value: float64(mem.TotalBytes)}, {Name: "memory_used_bytes", Value: float64(mem.UsedBytes)}, {Name: "load_1", Value: load[0]}, {Name: "load_5", Value: load[1]}, {Name: "load_15", Value: load[2]}, {Name: "uptime_seconds", Value: uptime}}
	for _, n := range network {
		metrics = append(metrics, &devopsv1.Metric{Name: "network_receive_bytes", Value: float64(n.ReceiveBytes), Labels: map[string]string{"interface": n.Name}}, &devopsv1.Metric{Name: "network_transmit_bytes", Value: float64(n.TransmitBytes), Labels: map[string]string{"interface": n.Name}})
	}
	hostname, _ := read("etc/hostname")
	osRelease, _ := read("etc/os-release")
	kernel, _ := read("proc/sys/kernel/osrelease")
	boot, _ := read("proc/sys/kernel/random/boot_id")
	return &devopsv1.MetricsSnapshot{SampledUnixMs: now().UnixMilli(), Metrics: metrics}, Inventory{Hostname: strings.TrimSpace(string(hostname)), OS: osID(string(osRelease)), Kernel: strings.TrimSpace(string(kernel)), Architecture: runtime.GOARCH, BootID: strings.TrimSpace(string(boot))}, nil
}
func osID(data string) string {
	for _, line := range strings.Split(data, "\n") {
		if value, ok := strings.CutPrefix(line, "ID="); ok {
			return strings.Trim(value, "\"")
		}
	}
	return "unknown"
}

type Buffer struct {
	mu    sync.Mutex
	max   int
	items []any
}

func NewBuffer(max int) *Buffer {
	if max < 1 {
		max = 1
	}
	return &Buffer{max: max}
}
func (b *Buffer) Push(value any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.items) == b.max {
		copy(b.items, b.items[1:])
		b.items = b.items[:b.max-1]
	}
	b.items = append(b.items, value)
}
func (b *Buffer) Drain() []any {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.items) == 0 {
		return nil
	}
	out := append([]any(nil), b.items...)
	b.items = nil
	return out
}

var ErrCollection = errors.New("metrics collection failed")
