package main

import (
	"fmt"
	"os"

	"mangahub/internal/tui"
)

func main() {
	if err := tui.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running MangaHub TUI: %v\n", err)
		os.Exit(1)
	}
}
