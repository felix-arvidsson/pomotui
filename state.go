package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	modePomo = iota
	modeShort
	modeLong
)

type Task struct {
	Title string `json:"title"`
	Est   int    `json:"est"`
	Act   int    `json:"act"`
	Done  bool   `json:"done"`
}

type Settings struct {
	Pomodoro     int  `json:"pomodoro"` // minuter
	Short        int  `json:"short"`
	Long         int  `json:"long"`
	LongInterval int  `json:"long_interval"`
	AutoBreaks   bool `json:"auto_breaks"`
	AutoPomos    bool `json:"auto_pomos"`
}

type State struct {
	Settings Settings      `json:"settings"`
	Tasks    []Task        `json:"tasks"`
	Active   int           `json:"active"`
	Mode     int           `json:"mode"`
	Round    int           `json:"round"` // avklarade pomodoros
	Running  bool          `json:"running"`
	Left     time.Duration `json:"left"`
	EndAt    time.Time     `json:"end_at"`
}

// Req är det klienten skickar till daemonen, en JSON-rad per anslutning.
type Req struct {
	Cmd string `json:"cmd"`
	I   int    `json:"i,omitempty"`
	N   int    `json:"n,omitempty"`
	S   string `json:"s,omitempty"`
}

var settingKeys = []string{"pomodoro", "short", "long", "interval", "autobreaks", "autopomos"}

func defaultState() State {
	s := State{Settings: Settings{25, 5, 15, 4, false, false}, Active: -1}
	s.setMode(modePomo)
	return s
}

func (s *State) dur(m int) time.Duration {
	min := [...]int{s.Settings.Pomodoro, s.Settings.Short, s.Settings.Long}[m]
	return time.Duration(min) * time.Minute
}

func (s *State) left() time.Duration {
	if s.Running {
		return max(time.Until(s.EndAt), 0)
	}
	return s.Left
}

func (s *State) setMode(m int) {
	s.Mode, s.Left, s.Running = m, s.dur(m), false
}

func (s *State) start() {
	s.EndAt, s.Running = time.Now().Add(s.Left), true
}

// finish avslutar nuvarande period och går vidare till nästa, som på pomofocus.
func (s *State) finish() {
	if s.Mode == modePomo {
		s.Round++
		if s.valid(s.Active) {
			s.Tasks[s.Active].Act++
		}
		if s.Round%max(s.Settings.LongInterval, 1) == 0 {
			s.setMode(modeLong)
		} else {
			s.setMode(modeShort)
		}
		if s.Settings.AutoBreaks {
			s.start()
		}
		return
	}
	s.setMode(modePomo)
	if s.Settings.AutoPomos {
		s.start()
	}
}

func (s *State) valid(i int) bool { return i >= 0 && i < len(s.Tasks) }

func (s *State) apply(r Req) {
	switch r.Cmd {
	case "toggle":
		if s.Running {
			s.Left, s.Running = s.left(), false
		} else {
			s.start()
		}
	case "skip":
		s.finish()
	case "reset":
		s.setMode(s.Mode)
	case "mode":
		if r.N >= modePomo && r.N <= modeLong {
			s.setMode(r.N)
		}
	case "add":
		if t := strings.TrimSpace(r.S); t != "" {
			s.Tasks = append(s.Tasks, Task{Title: t, Est: 1})
			if s.Active < 0 {
				s.Active = len(s.Tasks) - 1
			}
		}
	case "del":
		if s.valid(r.I) {
			s.Tasks = slices.Delete(s.Tasks, r.I, r.I+1)
			if s.Active == r.I {
				s.Active = -1
			} else if s.Active > r.I {
				s.Active--
			}
		}
	case "done":
		if s.valid(r.I) {
			s.Tasks[r.I].Done = !s.Tasks[r.I].Done
		}
	case "active":
		if s.valid(r.I) {
			s.Active = r.I
		}
	case "est":
		if s.valid(r.I) {
			s.Tasks[r.I].Est = max(s.Tasks[r.I].Est+r.N, 1)
		}
	case "set":
		st := &s.Settings
		switch r.S {
		case "pomodoro":
			st.Pomodoro = max(r.N, 1)
		case "short":
			st.Short = max(r.N, 1)
		case "long":
			st.Long = max(r.N, 1)
		case "interval":
			st.LongInterval = max(r.N, 1)
		case "autobreaks":
			st.AutoBreaks = r.N != 0
		case "autopomos":
			st.AutoPomos = r.N != 0
		}
		if !s.Running {
			s.setMode(s.Mode)
		}
	}
}

func statePath() string {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "pomo", "state.json")
}

func loadState() State {
	s := defaultState()
	if b, err := os.ReadFile(statePath()); err == nil {
		json.Unmarshal(b, &s)
	}
	return s
}

func (s *State) save() error {
	p := statePath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	if err := os.WriteFile(p+".tmp", b, 0o644); err != nil {
		return err
	}
	return os.Rename(p+".tmp", p)
}
