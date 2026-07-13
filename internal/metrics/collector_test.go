package metrics_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"silent-devops/internal/metrics"
)

func TestCollectorFromFixture(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		path := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(path), 0755)
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("proc/stat", "cpu 10 0 5 80 5 0 0 0 0 0\n")
	write("proc/meminfo", "MemTotal: 1000 kB\nMemAvailable: 400 kB\n")
	write("proc/loadavg", "1.0 2.0 3.0 1/1 1\n")
	write("proc/uptime", "50.0 20.0\n")
	write("proc/net/dev", "eth0: 100 1 0 0 0 0 0 0 200 1 0 0 0 0 0 0\n")
	write("etc/hostname", "host\n")
	write("etc/os-release", "ID=testos\n")
	write("proc/sys/kernel/osrelease", "1.2.3\n")
	write("proc/sys/kernel/random/boot_id", "boot\n")
	c := metrics.Collector{Root: root, MaxInterfaces: 8, Now: func() time.Time { return time.Unix(1, 0) }}
	snapshot, inventory, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Hostname != "host" || inventory.OS != "testos" || snapshot.SampledUnixMs != 1000 {
		t.Fatalf("snapshot=%+v inventory=%+v", snapshot, inventory)
	}
	if len(snapshot.Metrics) < 8 {
		t.Fatalf("metrics=%d", len(snapshot.Metrics))
	}
}
func TestBufferBoundsAndDrain(t *testing.T) {
	b := metrics.NewBuffer(2)
	b.Push(1)
	b.Push(2)
	b.Push(3)
	got := b.Drain()
	if len(got) != 2 || got[0].(int) != 2 || got[1].(int) != 3 {
		t.Fatalf("got=%v", got)
	}
	if len(b.Drain()) != 0 {
		t.Fatal("buffer not drained")
	}
}
