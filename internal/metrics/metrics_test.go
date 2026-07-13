package metrics_test

import (
	"math"
	"testing"
	"time"

	"silent-devops/internal/metrics"
)

func TestParseProcStatAndCounterReset(t *testing.T) {
	cpu, err := metrics.ParseCPU([]byte("cpu  100 2 30 400 5 0 0 0 0 0\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cpu.Total != 537 || cpu.Idle != 405 {
		t.Fatalf("cpu=%+v", cpu)
	}
	if _, ok := metrics.Rate(100, 90, time.Second); ok {
		t.Fatal("counter reset produced rate")
	}
	if rate, ok := metrics.Rate(100, 160, 2*time.Second); !ok || rate != 30 {
		t.Fatalf("rate=%v ok=%v", rate, ok)
	}
}
func TestParseMemoryLoadAndUptime(t *testing.T) {
	m, err := metrics.ParseMeminfo([]byte("MemTotal: 1000 kB\nMemAvailable: 250 kB\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.TotalBytes != 1024000 || m.UsedBytes != 768000 {
		t.Fatalf("memory=%+v", m)
	}
	load, err := metrics.ParseLoad([]byte("0.10 0.20 0.30 1/100 1\n"))
	if err != nil || load[2] != 0.3 {
		t.Fatalf("load=%v err=%v", load, err)
	}
	up, err := metrics.ParseUptime([]byte("123.45 20.0\n"))
	if err != nil || up != 123.45 {
		t.Fatalf("uptime=%v err=%v", up, err)
	}
}
func TestParseNetworkBoundsAndOverflow(t *testing.T) {
	data := []byte("Inter-| Receive | Transmit\n face |bytes packets errs drop fifo frame compressed multicast|bytes packets errs drop fifo colls carrier compressed\n eth0: 100 2 0 0 0 0 0 0 200 3 0 0 0 0 0 0\n lo: 18446744073709551615 1 0 0 0 0 0 0 1 1 0 0 0 0 0 0\n")
	n, err := metrics.ParseNetwork(data, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(n) != 1 || n[0].Name != "eth0" {
		t.Fatalf("network=%+v", n)
	}
	if math.IsInf(float64(n[0].ReceiveBytes), 0) {
		t.Fatal("overflow")
	}
}
func TestPseudoFilesystemFilter(t *testing.T) {
	for _, fs := range []string{"proc", "sysfs", "tmpfs", "cgroup2"} {
		if metrics.IncludeFilesystem(fs) {
			t.Fatalf("included %s", fs)
		}
	}
	if !metrics.IncludeFilesystem("ext4") {
		t.Fatal("ext4 excluded")
	}
}
