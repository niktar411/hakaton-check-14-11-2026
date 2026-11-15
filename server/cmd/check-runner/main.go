package main

import (
	"os"

	"hackathon/internal/infra/worker"
)

func main() {
	code := 2
	if len(os.Args) == 2 {
		code = worker.RunCheck(os.Args[1])
	}
	os.Exit(code)
}
