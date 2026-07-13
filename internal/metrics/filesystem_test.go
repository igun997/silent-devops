package metrics_test

import (
	"testing"

	"silent-devops/internal/metrics"
)

func TestParseMountsEscapesFiltersAndBounds(t *testing.T) {
	data := []byte("/dev/sda1 / ext4 rw 0 0\nproc /proc proc rw 0 0\n/dev/sdb1 /data\\040disk xfs rw 0 0\n/dev/sdc1 /extra ext4 rw 0 0\n")
	mounts, err := metrics.ParseMounts(data, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 2 || mounts[1].Path != "/data disk" {
		t.Fatalf("mounts=%+v", mounts)
	}
}
func TestFilesystemUsageOverflowSafe(t *testing.T) {
	usage, ok := metrics.UsageFromBlocks(100, 40, 4096)
	if !ok || usage.TotalBytes != 409600 || usage.UsedBytes != 245760 {
		t.Fatalf("usage=%+v ok=%v", usage, ok)
	}
	if _, ok := metrics.UsageFromBlocks(^uint64(0), 0, 4096); ok {
		t.Fatal("overflow accepted")
	}
}
