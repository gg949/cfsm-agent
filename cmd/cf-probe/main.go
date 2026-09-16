package main

import (
	"fmt"
	"os"

	"github.com/huilang-me/cfsm-agent/internal/cfprobe"
)

var version = "dev"

func main() {
	if err := cfprobe.Execute(os.Args[1:], version); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
		os.Exit(1)
	}
}
