package ops

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os/exec"
	"time"
)

func randomHex(size int) string {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "000000"
	}
	return hex.EncodeToString(buffer)
}

func runAgentCommand(ctx context.Context, args []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, args[0], args[1:]...).Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}
