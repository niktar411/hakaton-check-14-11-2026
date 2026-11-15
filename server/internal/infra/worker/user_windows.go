//go:build windows

package worker

func containerUser() string { return "10001:10001" }
