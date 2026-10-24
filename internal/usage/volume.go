// Package usage lists mounted filesystems and ZFS pools for the OmaUse terminal UI.
package usage

import (
	"cmp"
	"fmt"
	"math"
	"sort"
	"strings"
)

const BarWidth = 20

// Column is a sortable table column.
type Column int

const (
	ColName Column = iota
	ColKind
	ColWhere
	ColTotal
	ColUsed
	ColFree
	ColUse
	ColBar
	ColHealth
	colCount
)

// Tone is the fullness color for a percentage and its bar.
type Tone int

const (
	ToneNone Tone = iota
	ToneGreen
	ToneYellow
	ToneRed
)

// Volume is one filesystem or ZFS pool.
type Volume struct {
	Name    string
	Kind    string
	Where   string
	Total   uint64
	Used    uint64
	Free    uint64
	UsePct  float64
	HasSize bool
	Health  string
}

// FormatBytes renders a byte count as 812M, 7.2T, or 14.5T.
// ceil matches df, which rounds sizes up. Pools use nearest, which matches zpool list.
func FormatBytes(n uint64, ceil bool) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	units := []byte{'K', 'M', 'G', 'T', 'P'}
	val := float64(n)
	exp := -1
	for val >= 1024 && exp < len(units)-1 {
		val /= 1024
		exp++
	}
	for {
		decimals := bytePrecision(val, ceil)
		val = stepBytes(val, decimals, ceil)
		if val >= 1024 && exp < len(units)-1 {
			val /= 1024
			exp++
			continue
		}
		if bytePrecision(val, ceil) == 1 {
			return fmt.Sprintf("%.1f%c", val, units[exp])
		}
		return fmt.Sprintf("%.0f%c", val, units[exp])
	}
}

// bytePrecision keeps one decimal below 10, and also for mid-size pool
// numbers such as 14.5T. df rounds those up to a whole unit instead.
func bytePrecision(val float64, ceil bool) int {
	if val < 9.95 || (!ceil && val < 100 && math.Abs(val-math.Round(val)) >= 0.05) {
		return 1
	}
	return 0
}

func stepBytes(val float64, decimals int, ceil bool) float64 {
	factor := math.Pow(10, float64(decimals))
	scaled := val * factor
	var stepped float64
	if ceil {
		nearest := math.Round(scaled)
		if math.Abs(scaled-nearest) < 1e-4 {
			stepped = nearest
		} else {
			stepped = math.Ceil(scaled - 1e-9)
		}
	} else {
		stepped = math.Round(scaled)
	}
	return stepped / factor
}

// FormatPercent renders a rounded percentage, or "-" when size is unknown.
func FormatPercent(has bool, pct float64) string {
	if !has {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", pct)
}

// ToneFor colors percentages: green under 70, yellow from 70 to 89, red at 90 and above.
func ToneFor(has bool, pct float64) Tone {
	if !has {
		return ToneNone
	}
	p := math.Round(pct)
	switch {
	case p >= 90:
		return ToneRed
	case p >= 70:
		return ToneYellow
	default:
		return ToneGreen
	}
}

// BarFilled is the number of filled cells in a BarWidth bar.
// The fill matches the rounded percentage that FormatPercent prints.
func BarFilled(has bool, pct float64) int {
	if !has {
		return 0
	}
	p := math.Round(pct)
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	n := int(math.Round(p / 100 * float64(BarWidth)))
	if n > BarWidth {
		return BarWidth
	}
	if n < 0 {
		return 0
	}
	return n
}

// SortVolumes orders rows by column. Unknown sizes stay last on numeric columns.
func SortVolumes(rows []Volume, col Column, desc bool) {
	sort.SliceStable(rows, func(i, j int) bool {
		return before(rows[i], rows[j], col, desc)
	})
}

func before(a, b Volume, col Column, desc bool) bool {
	if numeric(col) {
		aMiss := !a.HasSize
		bMiss := !b.HasSize
		if aMiss || bMiss {
			if aMiss && bMiss {
				return false
			}
			return !aMiss
		}
	}
	c := compare(a, b, col)
	if c == 0 {
		return false
	}
	if desc {
		return c > 0
	}
	return c < 0
}

func numeric(col Column) bool {
	switch col {
	case ColTotal, ColUsed, ColFree, ColUse, ColBar:
		return true
	default:
		return false
	}
}

func compare(a, b Volume, col Column) int {
	switch col {
	case ColName:
		return strings.Compare(a.Name, b.Name)
	case ColKind:
		return strings.Compare(a.Kind, b.Kind)
	case ColWhere:
		return strings.Compare(a.Where, b.Where)
	case ColTotal:
		return cmp.Compare(a.Total, b.Total)
	case ColUsed:
		return cmp.Compare(a.Used, b.Used)
	case ColFree:
		return cmp.Compare(a.Free, b.Free)
	case ColUse, ColBar:
		return cmp.Compare(a.UsePct, b.UsePct)
	case ColHealth:
		return strings.Compare(a.Health, b.Health)
	default:
		return 0
	}
}
