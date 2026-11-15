//go:build !windows

package worker

import (
	"fmt"
	"os"
)

// containerUser runs check containers with the host worker identity so the
// read-only workspace and the review bind mount need no world permissions.
func containerUser() string {
	return fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
}
