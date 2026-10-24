package usage

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	minInterval = 1
	maxInterval = 60
	keyFooter   = "←→ column   s reverse   space pause   +/- interval   r refresh   q quit"
)

var colTitle = []string{
	"NAME",
	"KIND",
	"WHERE",
	"TOTAL",
	"USED",
	"FREE",
	"USE%",
	"BAR",
	"HEALTH",
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true)
	headerStyle = lipgloss.NewStyle().Bold(true)
	faintStyle  = lipgloss.NewStyle().Faint(true)
	greenStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	yellowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	redStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

type Model struct {
	rows       []Volume
	errText    string
	sortCol    Column
	sortDesc   bool
	seconds    int
	paused     bool
	tickGen    int
	collecting bool
	width      int
	height     int
}

type refreshMsg struct {
	volumes []Volume
	errText string
}

type tickMsg struct {
	gen int
}

// NewModel starts sorted by use percent, fullest first, refreshing every 2 seconds.
func NewModel() Model {
	return Model{
		sortCol:  ColUse,
		sortDesc: true,
		seconds:  2,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(collectCmd(), m.schedule())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case refreshMsg:
		m.rows = msg.volumes
		m.errText = msg.errText
		m.collecting = false
		SortVolumes(m.rows, m.sortCol, m.sortDesc)
		return m, nil
	case tickMsg:
		if msg.gen != m.tickGen || m.paused {
			return m, nil
		}
		cmds := []tea.Cmd{m.schedule()}
		if !m.collecting {
			m.collecting = true
			cmds = append(cmds, collectCmd())
		}
		return m, tea.Batch(cmds...)
	case tea.KeyMsg:
		cmd := m.handleKey(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "q", "ctrl+c":
		return tea.Quit
	case "left":
		m.move(-1)
	case "right":
		m.move(1)
	case "s":
		m.sortDesc = !m.sortDesc
		SortVolumes(m.rows, m.sortCol, m.sortDesc)
	case " ":
		m.paused = !m.paused
		m.tickGen++
		if !m.paused {
			m.collecting = true
			return tea.Batch(collectCmd(), m.schedule())
		}
	case "+", "=":
		if m.seconds < maxInterval {
			m.seconds++
		}
		return m.reschedule()
	case "-", "_":
		if m.seconds > minInterval {
			m.seconds--
		}
		return m.reschedule()
	case "r":
		m.collecting = true
		return collectCmd()
	}
	return nil
}

func (m *Model) move(delta int) {
	n := int(colCount)
	m.sortCol = Column((int(m.sortCol) + delta%n + n) % n)
	m.sortDesc = m.sortCol == ColUse
	SortVolumes(m.rows, m.sortCol, m.sortDesc)
}

func (m *Model) reschedule() tea.Cmd {
	if m.paused {
		return nil
	}
	m.tickGen++
	return m.schedule()
}

func (m Model) schedule() tea.Cmd {
	gen := m.tickGen
	d := time.Duration(m.seconds) * time.Second
	return tea.Tick(d, func(time.Time) tea.Msg {
		return tickMsg{gen: gen}
	})
}

// FitSize is the character grid that holds the current table with no spare rows.
func FitSize(volumes []Volume, errText string) (cols, rows int) {
	m := NewModel()
	m.rows = append([]Volume(nil), volumes...)
	m.errText = errText
	SortVolumes(m.rows, m.sortCol, m.sortDesc)
	return m.contentSize()
}

func (m Model) contentSize() (cols, rows int) {
	m.width = 0
	m.height = 0
	view := strings.TrimSuffix(m.View(), "\n")
	if view == "" {
		return 1, 1
	}
	lines := strings.Split(view, "\n")
	rows = len(lines)
	for _, line := range lines {
		if w := lipgloss.Width(line); w > cols {
			cols = w
		}
	}
	if cols < 1 {
		cols = 1
	}
	return cols, rows
}

func collectCmd() tea.Cmd {
	return func() tea.Msg {
		snap := Collect()
		return refreshMsg{volumes: snap.Volumes, errText: snap.Err}
	}
}

func (m Model) View() string {
	width := m.width
	lines := []string{m.titleLine(width)}
	if m.errText != "" {
		lines = append(lines, truncateEnd(m.errText, usable(width)))
	}
	lines = append(lines, "")
	header, rows := m.table(width)
	lines = append(lines, header)
	foot := []string{"", faintStyle.Render(keyFooter)}
	limit := len(rows)
	if m.height > 0 {
		room := m.height - len(lines) - len(foot)
		if room < 0 {
			room = 0
		}
		if room < len(rows) {
			if room >= 2 {
				limit = room - 1
			} else {
				limit = room
			}
		}
	}
	if len(rows) == 0 {
		lines = append(lines, faintStyle.Render("no drives found"))
	} else {
		lines = append(lines, rows[:limit]...)
		if limit < len(rows) {
			lines = append(lines, faintStyle.Render(fmt.Sprintf("and %d more", len(rows)-limit)))
		}
	}
	lines = append(lines, foot...)
	// No trailing newline: an extra one scrolls the title off a window
	// sized to this exact number of rows.
	return strings.Join(lines, "\n")
}

func (m Model) titleLine(width int) string {
	title := titleStyle.Render("OmaUse")
	status := fmt.Sprintf("refresh %ds", m.seconds)
	if m.paused {
		status = "paused"
	}
	status = faintStyle.Render(status)
	if width <= 0 {
		return title + "  " + status
	}
	gap := width - lipgloss.Width(title) - lipgloss.Width(status)
	if gap < 1 {
		gap = 1
	}
	return title + strings.Repeat(" ", gap) + status
}

func (m Model) table(width int) (string, []string) {
	widths := make([]int, colCount)
	for c := Column(0); c < colCount; c++ {
		widths[c] = lipgloss.Width(headerLabel(c, false, false))
		if c == ColBar && widths[c] < BarWidth {
			widths[c] = BarWidth
		}
		for _, v := range m.rows {
			if c == ColBar {
				continue
			}
			if w := lipgloss.Width(cellText(v, c)); w > widths[c] {
				widths[c] = w
			}
		}
	}
	shrinkTo(widths, usable(width))

	headerCells := make([]string, colCount)
	for c := Column(0); c < colCount; c++ {
		label := headerLabel(c, c == m.sortCol, m.sortDesc)
		headerCells[c] = place(label, widths[c], colRight(c), true)
	}
	header := headerStyle.Render(strings.Join(headerCells, "  "))

	rows := make([]string, len(m.rows))
	for i, v := range m.rows {
		cells := make([]string, colCount)
		for c := Column(0); c < colCount; c++ {
			switch c {
			case ColBar:
				cells[c] = padRightANSI(renderBar(v), widths[c])
			case ColUse:
				cells[c] = renderPercent(v, widths[c])
			case ColWhere:
				cells[c] = place(cellText(v, c), widths[c], false, false)
			default:
				cells[c] = place(cellText(v, c), widths[c], colRight(c), true)
			}
		}
		rows[i] = strings.Join(cells, "  ")
	}
	return header, rows
}

func shrinkTo(widths []int, limit int) {
	if limit <= 0 {
		return
	}
	order := []struct {
		col Column
		min int
	}{
		{ColWhere, 6},
		{ColName, 4},
		{ColKind, 4},
		{ColHealth, 1},
	}
	for tableWidth(widths) > limit {
		shrunk := false
		for _, item := range order {
			if widths[item.col] > item.min {
				widths[item.col]--
				shrunk = true
				break
			}
		}
		if !shrunk {
			return
		}
	}
}

func tableWidth(widths []int) int {
	n := 0
	for _, w := range widths {
		n += w
	}
	if len(widths) > 1 {
		n += 2 * (len(widths) - 1)
	}
	return n
}

func headerLabel(col Column, active, desc bool) string {
	title := colTitle[col]
	if active {
		if desc {
			return title + "▼"
		}
		return title + "▲"
	}
	return title + " "
}

func colRight(col Column) bool {
	switch col {
	case ColTotal, ColUsed, ColFree, ColUse:
		return true
	default:
		return false
	}
}

func cellText(v Volume, col Column) string {
	switch col {
	case ColName:
		return v.Name
	case ColKind:
		return v.Kind
	case ColWhere:
		return v.Where
	case ColTotal:
		return sizeText(v, v.Total)
	case ColUsed:
		return sizeText(v, v.Used)
	case ColFree:
		return sizeText(v, v.Free)
	case ColUse:
		return FormatPercent(v.HasSize, v.UsePct)
	case ColHealth:
		if v.Health == "" {
			return "-"
		}
		return v.Health
	default:
		return ""
	}
}

func sizeText(v Volume, n uint64) string {
	if !v.HasSize {
		return "-"
	}
	return FormatBytes(n, v.Kind != "zfs pool")
}

func renderPercent(v Volume, width int) string {
	text := FormatPercent(v.HasSize, v.UsePct)
	if v.HasSize {
		text = toneStyle(ToneFor(true, v.UsePct)).Render(text)
	}
	return padLeftANSI(text, width)
}

// barGlyph is a lower seven-eighths block. The empty eighth is a few pixels of
// space between one row's bar and the next.
const barGlyph = "▇"

func renderBar(v Volume) string {
	filled := BarFilled(v.HasSize, v.UsePct)
	empty := BarWidth - filled
	if !v.HasSize {
		return faintStyle.Render(strings.Repeat(barGlyph, BarWidth))
	}
	body := toneStyle(ToneFor(true, v.UsePct)).Render(strings.Repeat(barGlyph, filled))
	return body + faintStyle.Render(strings.Repeat(barGlyph, empty))
}

func toneStyle(t Tone) lipgloss.Style {
	switch t {
	case ToneYellow:
		return yellowStyle
	case ToneRed:
		return redStyle
	case ToneGreen:
		return greenStyle
	default:
		return lipgloss.NewStyle()
	}
}

func place(s string, width int, right, trimEnd bool) string {
	if lipgloss.Width(s) > width {
		if trimEnd {
			s = truncateEnd(s, width)
		} else {
			s = truncateStart(s, width)
		}
	}
	if right {
		return padLeftANSI(s, width)
	}
	return padRightANSI(s, width)
}

func padLeftANSI(s string, width int) string {
	pad := width - lipgloss.Width(s)
	if pad <= 0 {
		return s
	}
	return strings.Repeat(" ", pad) + s
}

func padRightANSI(s string, width int) string {
	pad := width - lipgloss.Width(s)
	if pad <= 0 {
		return s
	}
	return s + strings.Repeat(" ", pad)
}

func truncateEnd(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	budget := width - 1
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if used+rw > budget {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String() + "…"
}

func truncateStart(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	budget := width - 1
	runes := []rune(s)
	used := 0
	i := len(runes)
	for i > 0 {
		rw := lipgloss.Width(string(runes[i-1]))
		if used+rw > budget {
			break
		}
		used += rw
		i--
	}
	return "…" + string(runes[i:])
}

func usable(width int) int {
	if width <= 0 {
		return 4096
	}
	return width
}
