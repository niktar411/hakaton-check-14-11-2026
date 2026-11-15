package metrics

import (
	"strings"
	"testing"
)

func TestNumberAndPercent(t *testing.T) {
	numbers := map[string]float64{
		"0B": 0, "512B": 512, "1.5MiB": 1.5 * 1024 * 1024, "2GB": 2e9,
		"3GiB": 3 * 1024 * 1024 * 1024, "bad": 0, "": 0,
	}
	for input, want := range numbers {
		if got := Number(input); got != want {
			t.Fatalf("Number(%q) = %v, want %v", input, got, want)
		}
	}
	percents := map[string]float64{"12.34%": 12.34, "-1%": 0, "abc": 0, "0%": 0}
	for input, want := range percents {
		if got := Percent(input); got != want {
			t.Fatalf("Percent(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestCollectIgnoresForeignIdentifiersAndMapsStats(t *testing.T) {
	runner := func(args []string) (string, error) {
		switch {
		case len(args) == 5 && args[1] == "ps":
			return "aaaaaaaaaaaa\nnot-an-id\nbbbbbbbbbbbb\n", nil
		case len(args) > 3 && args[1] == "inspect":
			if len(args) != 6 {
				t.Fatalf("inspect identifiers = %v", args[4:])
			}
			return "/web|hackathon-web:1|running|healthy|2\n/app|hackathon-api:1|exited|none|0\n", nil
		case len(args) > 2 && args[1] == "stats":
			return "web|12.5%|100MiB / 256MiB\n", nil
		default:
			t.Fatalf("unexpected command: %v", args)
			return "", nil
		}
	}
	containers, err := Collect("server1", runner)
	if err != nil {
		t.Fatal(err)
	}
	if len(containers) != 2 {
		t.Fatalf("containers = %d", len(containers))
	}
	web := containers[0]
	if web.Name != "web" || web.Server != "server1" || web.State != "running" || web.Health != "healthy" {
		t.Fatalf("web = %+v", web)
	}
	if web.CPUPercent != 12.5 || web.MemoryUsed != 100*1024*1024 || web.MemoryLimit != 256*1024*1024 {
		t.Fatalf("web stats = %+v", web)
	}
	if web.Restarts != 2 {
		t.Fatalf("restarts = %d", web.Restarts)
	}
	if containers[1].Health != "none" || containers[1].State != "exited" {
		t.Fatalf("api = %+v", containers[1])
	}
}

func TestCollectLimitsIdentifiersAndHandlesErrors(t *testing.T) {
	var seen []string
	runner := func(args []string) (string, error) {
		if args[1] == "ps" {
			return strings.Repeat("aaaaaaaaaaaa\n", 150), nil
		}
		if args[1] == "inspect" {
			seen = args[4:]
			return "", nil
		}
		return "", nil
	}
	if _, err := Collect("server1", runner); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 100 {
		t.Fatalf("inspect identifiers = %d, want 100", len(seen))
	}
}
