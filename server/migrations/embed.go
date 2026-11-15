package migrations

import "embed"

// Files is the immutable migration history included in the server binary.
//
//go:embed *.sql
var Files embed.FS
