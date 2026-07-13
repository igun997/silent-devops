package metrics

import (
	"bufio"
	"bytes"
	"errors"
	"strconv"
	"strings"
	"syscall"
)

type Mount struct{ Path, Type string }
type FilesystemUsage struct {
	Path, Type            string
	TotalBytes, UsedBytes uint64
}

func ParseMounts(data []byte, limit int) ([]Mount, error) {
	if limit <= 0 {
		return nil, errors.New("mount limit required")
	}
	var out []Mount
	s := bufio.NewScanner(bytes.NewReader(data))
	for s.Scan() {
		f := strings.Fields(s.Text())
		if len(f) < 3 {
			continue
		}
		if !IncludeFilesystem(f[2]) {
			continue
		}
		path := decodeMount(f[1])
		out = append(out, Mount{Path: path, Type: f[2]})
		if len(out) >= limit {
			break
		}
	}
	return out, s.Err()
}
func decodeMount(value string) string {
	return strings.NewReplacer("\\040", " ", "\\011", "\t", "\\012", "\n", "\\134", "\\").Replace(value)
}
func UsageFromBlocks(blocks, available, blockSize uint64) (FilesystemUsage, bool) {
	if available > blocks || blockSize != 0 && blocks > ^uint64(0)/blockSize {
		return FilesystemUsage{}, false
	}
	total := blocks * blockSize
	free := available * blockSize
	return FilesystemUsage{TotalBytes: total, UsedBytes: total - free}, true
}
func CollectFilesystems(mounts []Mount, max int) ([]FilesystemUsage, error) {
	var out []FilesystemUsage
	for _, mount := range mounts {
		if len(out) >= max {
			break
		}
		var stat syscall.Statfs_t
		if err := syscall.Statfs(mount.Path, &stat); err != nil {
			continue
		}
		usage, ok := UsageFromBlocks(stat.Blocks, stat.Bavail, uint64(stat.Bsize))
		if !ok {
			continue
		}
		usage.Path = mount.Path
		usage.Type = mount.Type
		out = append(out, usage)
	}
	return out, nil
}
func parseUint(value string) (uint64, error) { return strconv.ParseUint(value, 10, 64) }
