package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"usage/internal/quiet"
	"usage/internal/usage"
)

func main() {
	sizeOnly := false
	quietMode := false
	clearState := false
	for _, arg := range os.Args[1:] {
		switch arg {
		case "-s", "--size":
			sizeOnly = true
		case "-q", "--quiet":
			quietMode = true
		case "-c", "--clear", "--clear-state":
			clearState = true
		case "-h", "--help":
			printHelp()
			return
		default:
			fmt.Fprintf(os.Stderr, "omause: unknown argument %q\n", arg)
			printHelp()
			os.Exit(2)
		}
	}
	if sizeOnly && (quietMode || clearState) {
		fmt.Fprintln(os.Stderr, "omause: --size cannot be combined with --quiet or --clear")
		os.Exit(2)
	}
	if clearState {
		if err := quiet.ClearState(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("cleared threshold alert state")
		if !quietMode {
			return
		}
	}
	if sizeOnly {
		snap := usage.Collect()
		cols, rows := usage.FitSize(snap.Volumes, snap.Err)
		fmt.Printf("%d %d\n", cols, rows)
		return
	}
	if quietMode {
		if err := quiet.Run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	p := tea.NewProgram(usage.NewModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Fprintf(os.Stderr, `OmaUse - mounted filesystems and ZFS pool usage

Usage:
  omause [options]

Options:
  -c,  --clear    clear fired threshold alert state
  -h,  --help     show this help
  -q,  --quiet    collect once, apply threshold alerts, quit
  -s,  --size     print fitted terminal cols and rows

  -c -q           clear state, then run quiet mode

Quiet mode reads ~/.config/omause/config.json and stores fired
thresholds in ~/.local/state/omause/thresholds.json.
`)
}
