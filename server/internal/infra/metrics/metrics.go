// Package metrics collects audit-safe host and Docker container metrics.
package metrics

import (
	"regexp"
	"strconv"
	"strings"
)

// Container is one Docker container as exposed to the admin dashboard.
type Container struct {
	Server      string  `json:"server"`
	Name        string  `json:"name"`
	Image       string  `json:"image"`
	State       string  `json:"state"`
	Health      string  `json:"health"`
	CPUPercent  float64 `json:"cpuPercent"`
	MemoryUsed  int64   `json:"memoryUsed"`
	MemoryLimit int64   `json:"memoryLimit"`
	Restarts    int     `json:"restarts"`
}

// Host is one server measurement.
type Host struct {
	Name          string      `json:"name"`
	Available     bool        `json:"available"`
	CPUPercent    float64     `json:"cpuPercent"`
	MemoryUsed    int64       `json:"memoryUsed"`
	MemoryTotal   int64       `json:"memoryTotal"`
	DiskUsed      int64       `json:"diskUsed"`
	DiskTotal     int64       `json:"diskTotal"`
	UptimeSeconds float64     `json:"uptimeSeconds"`
	SampledAt     string      `json:"sampledAt"`
	Containers    []Container `json:"containers"`
}

// Runner executes one fixed command and returns its standard output.
type Runner func(args []string) (string, error)

var (
	identifier = regexp.MustCompile(`^[a-f0-9]{12,64}$`)
	quantity   = regexp.MustCompile(`^\s*([0-9]+(?:\.[0-9]+)?)\s*([kmgt]?i?b)?\s*$`)
)

var units = map[string]float64{
	"b": 1, "kb": 1000, "kib": 1024,
	"mb": 1000 * 1000, "mib": 1024 * 1024,
	"gb": 1000 * 1000 * 1000, "gib": 1024 * 1024 * 1024,
	"tb": 1000 * 1000 * 1000 * 1000, "tib": 1024 * 1024 * 1024 * 1024,
}

// Number converts a human readable Docker size such as "1.5MiB" to bytes.
func Number(value string) float64 {
	match := quantity.FindStringSubmatch(strings.ToLower(value))
	if match == nil {
		return 0
	}
	amount, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0
	}
	unit := match[2]
	if unit == "" {
		unit = "b"
	}
	factor, ok := units[unit]
	if !ok {
		factor = 1
	}
	return amount * factor
}

// Percent converts "12.34%" to a non-negative number.
func Percent(value string) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, "%")), 64)
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}

// Collect reads Docker containers through the fixed Docker CLI commands.
func Collect(server string, runner Runner) ([]Container, error) {
	out, err := runner([]string{"docker", "ps", "--all", "--format", "{{.ID}}"})
	if err != nil {
		return nil, err
	}
	identifiers := make([]string, 0, 16)
	for _, line := range strings.Split(out, "\n") {
		value := strings.TrimSpace(line)
		if identifier.MatchString(value) {
			identifiers = append(identifiers, value)
		}
		if len(identifiers) == 100 {
			break
		}
	}
	if len(identifiers) == 0 {
		return []Container{}, nil
	}

	inspectArgs := []string{
		"docker", "inspect", "--format",
		"{{.Name}}|{{.Config.Image}}|{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}|{{.RestartCount}}",
	}
	inspectArgs = append(inspectArgs, identifiers...)
	inspected, err := runner(inspectArgs)
	if err != nil {
		return nil, err
	}
	statsText, err := runner([]string{"docker", "stats", "--no-stream", "--format", "{{.Name}}|{{.CPUPerc}}|{{.MemUsage}}"})
	if err != nil {
		return nil, err
	}

	type usage struct {
		cpu       float64
		used, max int64
	}
	stats := map[string]usage{}
	for _, line := range strings.Split(statsText, "\n") {
		fields := strings.Split(line, "|")
		if len(fields) != 3 {
			continue
		}
		memory := strings.Split(fields[2], "/")
		var used, limit int64
		if len(memory) > 0 {
			used = int64(Number(memory[0]))
		}
		if len(memory) == 2 {
			limit = int64(Number(memory[1]))
		}
		stats[strings.TrimSpace(fields[0])] = usage{cpu: Percent(fields[1]), used: used, max: limit}
	}

	result := make([]Container, 0, len(identifiers))
	for _, line := range strings.Split(inspected, "\n") {
		fields := strings.Split(line, "|")
		if len(fields) != 5 {
			continue
		}
		name := strings.TrimPrefix(strings.TrimSpace(fields[0]), "/")
		if name == "" {
			continue
		}
		name = truncate(name, 128)
		stat := stats[name]
		restarts, err := strconv.Atoi(strings.TrimSpace(fields[4]))
		if err != nil || restarts < 0 {
			restarts = 0
		}
		result = append(result, Container{
			Server:      server,
			Name:        name,
			Image:       truncate(fields[1], 256),
			State:       truncate(fields[2], 32),
			Health:      truncate(fields[3], 32),
			CPUPercent:  stat.cpu,
			MemoryUsed:  stat.used,
			MemoryLimit: stat.max,
			Restarts:    restarts,
		})
	}
	return result, nil
}

func truncate(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	return value[:maximum]
}
