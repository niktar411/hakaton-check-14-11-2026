package worker

import (
	"crypto/rand"
	"encoding/base64"
	"regexp"
)

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

func randomToken(size int) string {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(buffer)
}
