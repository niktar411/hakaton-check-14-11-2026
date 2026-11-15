//go:build linux

package metrics

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// HostMetrics reads /proc and the root filesystem of the current host.
func HostMetrics(name string, containers []Container) (Host, error) {
	before, err := cpuSnapshot()
	if err != nil {
		return Host{}, err
	}
	// A real measurement interval rather than a simulated loading delay.
	time.Sleep(200 * time.Millisecond)
	after, err := cpuSnapshot()
	if err != nil {
		return Host{}, err
	}
	totalDelta := after.total - before.total
	cpu := 0.0
	if totalDelta > 0 {
		cpu = 100 * (1 - float64(after.idle-before.idle)/float64(totalDelta))
	}
	memory, err := memoryInfo()
	if err != nil {
		return Host{}, err
	}
	var storage syscall.Statfs_t
	if err := syscall.Statfs("/", &storage); err != nil {
		return Host{}, err
	}
	uptime, err := uptimeSeconds()
	if err != nil {
		return Host{}, err
	}
	return Host{
		Name:          name,
		Available:     true,
		CPUPercent:    cpu,
		MemoryUsed:    memory.total - memory.available,
		MemoryTotal:   memory.total,
		DiskUsed:      int64(storage.Blocks-storage.Bfree) * int64(storage.Bsize),
		DiskTotal:     int64(storage.Blocks) * int64(storage.Bsize),
		UptimeSeconds: uptime,
		SampledAt:     time.Now().UTC().Format(time.RFC3339),
		Containers:    containers,
	}, nil
}

type cpuSample struct {
	total int64
	idle  int64
}

func cpuSnapshot() (cpuSample, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return cpuSample{}, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return cpuSample{}, errors.New("proc stat is empty")
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) < 9 || fields[0] != "cpu" {
		return cpuSample{}, errors.New("proc stat is malformed")
	}
	values := make([]int64, 0, 8)
	for _, field := range fields[1:9] {
		value, err := strconv.ParseInt(field, 10, 64)
		if err != nil {
			return cpuSample{}, err
		}
		values = append(values, value)
	}
	var total int64
	for _, value := range values {
		total += value
	}
	return cpuSample{total: total, idle: values[3] + values[4]}, nil
}

func memoryInfo() (struct{ total, available int64 }, error) {
	result := struct{ total, available int64 }{}
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return result, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, found := strings.Cut(scanner.Text(), ":")
		if !found {
			continue
		}
		fields := strings.Fields(value)
		if len(fields) == 0 {
			continue
		}
		number, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "MemTotal":
			result.total = number * 1024
		case "MemAvailable":
			result.available = number * 1024
		}
	}
	return result, scanner.Err()
}

func uptimeSeconds() (float64, error) {
	content, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(content))
	if len(fields) == 0 {
		return 0, errors.New("proc uptime is empty")
	}
	return strconv.ParseFloat(fields[0], 64)
}
