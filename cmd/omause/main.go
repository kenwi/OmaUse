package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"usage/internal/usage"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--size" {
		snap := usage.Collect()
		cols, rows := usage.FitSize(snap.Volumes, snap.Err)
		fmt.Printf("%d %d\n", cols, rows)
		return
	}
	p := tea.NewProgram(usage.NewModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
