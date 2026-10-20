package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// pomofocus färger: bakgrund och mörkare nyans per läge
var (
	bgs   = [...]string{"#ba4949", "#38858a", "#397097"}
	darks = [...]string{"#a44141", "#2f7276", "#2f5f80"}
	white = lipgloss.Color("#ffffff")
)

const listW = 46

type tickMsg struct{}

func tick() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

type model struct {
	st     *State
	err    error
	cur    int
	adding bool
	input  textinput.Model
	w, h   int
}

func runTUI() {
	ti := textinput.New()
	ti.Placeholder = "Vad jobbar du med?"
	ti.CharLimit = 80
	m := &model{input: ti}
	m.do(Req{Cmd: "status"})
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		die("pomo: " + err.Error())
	}
	if m.err != nil {
		fmt.Fprintln(os.Stderr, "pomo:", m.err)
	}
}

func (m *model) do(r Req) {
	st, err := call(r, true)
	if err != nil {
		m.err = err
		return
	}
	m.st, m.err = st, nil
	m.cur = max(min(m.cur, len(st.Tasks)-1), 0)
}

func (m *model) Init() tea.Cmd { return tick() }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tickMsg:
		m.do(Req{Cmd: "status"})
		return m, tick()
	case tea.KeyMsg:
		if m.adding {
			switch msg.String() {
			case "enter":
				m.do(Req{Cmd: "add", S: m.input.Value()})
				fallthrough
			case "esc":
				m.adding = false
				m.input.Reset()
				m.input.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "Q":
			call(Req{Cmd: "quit"}, false)
			return m, tea.Quit
		case " ":
			m.do(Req{Cmd: "toggle"})
		case "s":
			m.do(Req{Cmd: "skip"})
		case "r":
			m.do(Req{Cmd: "reset"})
		case "1", "2", "3":
			m.do(Req{Cmd: "mode", N: int(msg.String()[0] - '1')})
		case "a":
			m.adding = true
			return m, m.input.Focus()
		case "j", "down":
			if m.st != nil && m.cur < len(m.st.Tasks)-1 {
				m.cur++
			}
		case "k", "up":
			m.cur = max(m.cur-1, 0)
		case "enter":
			m.do(Req{Cmd: "active", I: m.cur})
		case "d":
			m.do(Req{Cmd: "done", I: m.cur})
		case "x":
			m.do(Req{Cmd: "del", I: m.cur})
		case "+", "=":
			m.do(Req{Cmd: "est", I: m.cur, N: 1})
		case "-":
			m.do(Req{Cmd: "est", I: m.cur, N: -1})
		}
	}
	return m, nil
}

func (m *model) View() string {
	if m.st == nil {
		return fmt.Sprintf("pomo: kan inte nå daemonen: %v\n", m.err)
	}
	s := m.st
	bg, dark := lipgloss.Color(bgs[s.Mode]), lipgloss.Color(darks[s.Mode])
	base := lipgloss.NewStyle().Background(bg).Foreground(white)
	faint := base.Foreground(lipgloss.Color("#f0d0d0"))

	var lines []string
	add := func(str string) { lines = append(lines, strings.Split(str, "\n")...) }
	gap := func() { lines = append(lines, "") }

	var tabs []string
	for i, name := range []string{"Pomodoro", "Kort paus", "Lång paus"} {
		t := base.Padding(0, 2)
		if i == s.Mode {
			t = t.Background(dark).Bold(true)
		}
		tabs = append(tabs, t.Render(name))
	}
	add(strings.Join(tabs, base.Render(" ")))
	gap()
	add(base.Bold(true).Render(bigClock(clock(s.Left))))
	gap()
	btn := "START"
	if s.Running {
		btn = "PAUS"
	}
	add(lipgloss.NewStyle().Background(white).Foreground(bg).Bold(true).Width(17).Align(lipgloss.Center).Render(btn))
	gap()
	msg := "Dags för paus!"
	if s.Mode == modePomo {
		msg = "Dags att fokusera!"
	}
	add(base.Render(fmt.Sprintf("#%d", s.Round+1)))
	if s.valid(s.Active) {
		msg = s.Tasks[s.Active].Title
	}
	add(base.Render(msg))
	gap()

	add(base.Bold(true).Width(listW).Render("Uppgifter"))
	add(faint.Render(strings.Repeat("─", listW)))
	for i, t := range s.Tasks {
		cursor, check := "  ", "○"
		if i == m.cur && !m.adding {
			cursor = "› "
		}
		if t.Done {
			check = "✓"
		}
		right := fmt.Sprintf("%d/%d", t.Act, t.Est)
		left := ansi.Truncate(cursor+check+" "+t.Title, listW-len(right)-1, "…")
		row := left + strings.Repeat(" ", listW-ansi.StringWidth(left)-len(right)) + right
		st := base
		if i == s.Active {
			st = st.Background(dark).Bold(true)
		}
		if t.Done {
			st = st.Strikethrough(true).Foreground(lipgloss.Color("#f0d0d0"))
		}
		add(st.Render(row))
	}
	if m.adding {
		m.input.PromptStyle, m.input.TextStyle = base, base
		m.input.PlaceholderStyle = faint
		m.input.Cursor.Style = base
		m.input.Cursor.TextStyle = base
		m.input.Width = listW - 4
		add(base.Width(listW).Render(base.Render("+ ") + m.input.View()))
	} else {
		add(faint.Width(listW).Render("+ lägg till uppgift (a)"))
	}
	if sum := s.summary(); sum != "" {
		gap()
		add(faint.Render(sum))
	}
	gap()
	add(faint.Render("space start/paus · s skippa · r nollställ · 1-3 läge"))
	add(faint.Render("a ny · j/k · enter aktiv · d klar · x ta bort · +/- est"))
	add(faint.Render("q stäng (timern fortsätter) · Q stoppa daemonen"))
	if m.err != nil {
		add(base.Bold(true).Render("fel: " + m.err.Error()))
	}

	ws := lipgloss.WithWhitespaceBackground(bg)
	blank := lipgloss.PlaceHorizontal(m.w, lipgloss.Left, "", ws)
	out := make([]string, 0, m.h)
	for range max((m.h-len(lines))/2, 0) {
		out = append(out, blank)
	}
	for _, l := range lines {
		out = append(out, lipgloss.PlaceHorizontal(m.w, lipgloss.Center, base.Render(l), ws))
	}
	for len(out) < m.h {
		out = append(out, blank)
	}
	return strings.Join(out, "\n")
}

// summary ger pomofocus "Pomos: x/y  Klar ca HH:MM".
func (s *State) summary() string {
	act, est, rest := 0, 0, 0
	for _, t := range s.Tasks {
		act += t.Act
		est += t.Est
		if !t.Done {
			rest += max(t.Est-t.Act, 0)
		}
	}
	if est == 0 {
		return ""
	}
	// shortcut: räknar alla pauser som korta, ignorerar långa pauser – räcker som uppskattning
	d := time.Duration(rest) * (s.dur(modePomo) + s.dur(modeShort))
	return fmt.Sprintf("Pomos: %d/%d   Klar ca %s (%.1fh)", act, est, time.Now().Add(d).Format("15:04"), d.Hours())
}

var font = map[rune][5]string{
	'0': {"###", "# #", "# #", "# #", "###"},
	'1': {" # ", "## ", " # ", " # ", "###"},
	'2': {"###", "  #", "###", "#  ", "###"},
	'3': {"###", "  #", "###", "  #", "###"},
	'4': {"# #", "# #", "###", "  #", "  #"},
	'5': {"###", "#  ", "###", "  #", "###"},
	'6': {"###", "#  ", "###", "# #", "###"},
	'7': {"###", "  #", "  #", "  #", "  #"},
	'8': {"###", "# #", "###", "# #", "###"},
	'9': {"###", "# #", "###", "  #", "###"},
	':': {" ", "#", " ", "#", " "},
}

func bigClock(txt string) string {
	rows := make([]string, 5)
	for r := range rows {
		var parts []string
		for _, ch := range txt {
			parts = append(parts, font[ch][r])
		}
		row := strings.Join(parts, " ")
		row = strings.ReplaceAll(row, "#", "██")
		rows[r] = strings.ReplaceAll(row, " ", "  ")
	}
	return strings.Join(rows, "\n")
}
