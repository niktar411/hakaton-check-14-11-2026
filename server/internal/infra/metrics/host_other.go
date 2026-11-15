//go:build !linux

package metrics

import "errors"

// HostMetrics is only implemented on the Linux servers.
func HostMetrics(string, []Container) (Host, error) {
	return Host{}, errors.New("host metrics require linux")
}
