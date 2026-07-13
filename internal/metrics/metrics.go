package metrics

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type CPU struct{ Total, Idle uint64 }

func ParseCPU(data []byte) (CPU, error) {
	line, _, ok := bytes.Cut(data, []byte{'\n'})
	if !ok {
		line = data
	}
	f := strings.Fields(string(line))
	if len(f) < 5 || f[0] != "cpu" {
		return CPU{}, errors.New("invalid proc stat")
	}
	var values []uint64
	for _, v := range f[1:] {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return CPU{}, err
		}
		values = append(values, n)
	}
	var total uint64
	for _, v := range values {
		if ^uint64(0)-total < v {
			return CPU{}, errors.New("CPU counter overflow")
		}
		total += v
	}
	return CPU{Total: total, Idle: values[3] + values[4]}, nil
}
func Rate(previous, current uint64, elapsed time.Duration) (float64, bool) {
	if current < previous || elapsed <= 0 {
		return 0, false
	}
	return float64(current-previous) / elapsed.Seconds(), true
}

type Memory struct{ TotalBytes, AvailableBytes, UsedBytes uint64 }

func ParseMeminfo(data []byte) (Memory, error) {
	var m Memory
	s := bufio.NewScanner(bytes.NewReader(data))
	for s.Scan() {
		f := strings.Fields(s.Text())
		if len(f) < 2 {
			continue
		}
		v, err := strconv.ParseUint(f[1], 10, 64)
		if err != nil {
			return m, err
		}
		if v > ^uint64(0)/1024 {
			return m, errors.New("memory overflow")
		}
		switch strings.TrimSuffix(f[0], ":") {
		case "MemTotal":
			m.TotalBytes = v * 1024
		case "MemAvailable":
			m.AvailableBytes = v * 1024
		}
	}
	if m.TotalBytes == 0 || m.AvailableBytes > m.TotalBytes {
		return m, errors.New("invalid meminfo")
	}
	m.UsedBytes = m.TotalBytes - m.AvailableBytes
	return m, s.Err()
}
func ParseLoad(data []byte) ([3]float64, error) {
	var out [3]float64
	f := strings.Fields(string(data))
	if len(f) < 3 {
		return out, errors.New("invalid loadavg")
	}
	for i := range 3 {
		v, err := strconv.ParseFloat(f[i], 64)
		if err != nil {
			return out, err
		}
		out[i] = v
	}
	return out, nil
}
func ParseUptime(data []byte) (float64, error) {
	f := strings.Fields(string(data))
	if len(f) < 1 {
		return 0, errors.New("invalid uptime")
	}
	return strconv.ParseFloat(f[0], 64)
}

type Network struct {
	Name                        string
	ReceiveBytes, TransmitBytes uint64
}

func ParseNetwork(data []byte, limit int) ([]Network, error) {
	if limit <= 0 {
		return nil, errors.New("network limit required")
	}
	var out []Network
	s := bufio.NewScanner(bytes.NewReader(data))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if !strings.Contains(line, ":") {
			continue
		}
		name, rest, _ := strings.Cut(line, ":")
		f := strings.Fields(rest)
		if len(f) < 16 {
			return nil, fmt.Errorf("invalid network row %q", name)
		}
		rx, err := strconv.ParseUint(f[0], 10, 64)
		if err != nil {
			return nil, err
		}
		tx, err := strconv.ParseUint(f[8], 10, 64)
		if err != nil {
			return nil, err
		}
		if name == "lo" {
			continue
		}
		out = append(out, Network{Name: name, ReceiveBytes: rx, TransmitBytes: tx})
		if len(out) >= limit {
			break
		}
	}
	return out, s.Err()
}

var pseudo = map[string]bool{"proc": true, "sysfs": true, "tmpfs": true, "devtmpfs": true, "devpts": true, "cgroup": true, "cgroup2": true, "overlay": true, "squashfs": true, "securityfs": true, "debugfs": true, "tracefs": true, "pstore": true, "configfs": true, "fusectl": true, "mqueue": true, "hugetlbfs": true, "rpc_pipefs": true, "autofs": true, "binfmt_misc": true}

func IncludeFilesystem(kind string) bool { return !pseudo[kind] }
