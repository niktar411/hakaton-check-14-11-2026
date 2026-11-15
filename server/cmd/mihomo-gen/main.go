package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"hackathon/internal/infra/mihomo"
)

func main() {
	standalone := flag.Bool("standalone", false, "write a complete profile instead of a provider include")
	output := flag.String("output", "", "destination file (default: team-vpn.local.yaml)")
	flag.Parse()

	inline := os.Getenv("MIHOMO_VPN_OUTBOUND_JSON")
	path := os.Getenv("MIHOMO_VPN_OUTBOUND_PATH")
	if (inline == "") == (path == "") {
		fail("configure exactly one local outbound source: MIHOMO_VPN_OUTBOUND_JSON or MIHOMO_VPN_OUTBOUND_PATH")
	}
	source := inline
	if path != "" {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			fail("local outbound include is not a regular file")
		}
		content, err := os.ReadFile(path)
		if err != nil {
			fail("local outbound include is unreadable")
		}
		source = string(content)
	}
	destination := *output
	if destination == "" {
		destination = "team-vpn.local.yaml"
	}
	if err := mihomo.Generate(source, *standalone, destination); err != nil {
		fail(err.Error())
	}
	_, _ = fmt.Fprintln(os.Stdout, "wrote", destination)
}

func fail(message string) {
	slog.Error("mihomo-gen failed", "reason", message)
	os.Exit(1)
}
