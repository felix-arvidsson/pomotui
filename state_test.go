package main

import (
	"testing"
	"time"
)

func TestCycle(t *testing.T) {
	s := defaultState()
	s.apply(Req{Cmd: "add", S: "skriva"})
	want := []int{modeShort, modePomo, modeShort, modePomo, modeShort, modePomo, modeLong, modePomo}
	for i, w := range want {
		s.apply(Req{Cmd: "skip"})
		if s.Mode != w {
			t.Fatalf("steg %d: läge %d, ville %d", i, s.Mode, w)
		}
	}
	if s.Round != 4 || s.Tasks[0].Act != 4 {
		t.Fatalf("round=%d act=%d, ville 4/4", s.Round, s.Tasks[0].Act)
	}
	if s.Running || s.Left != 25*time.Minute {
		t.Fatal("ska stå still på full pomodoro utan autostart")
	}
}

func TestToggleKeepsTime(t *testing.T) {
	s := defaultState()
	s.apply(Req{Cmd: "toggle"})
	s.EndAt = time.Now().Add(10 * time.Minute)
	s.apply(Req{Cmd: "toggle"})
	if s.Running || s.Left < 9*time.Minute || s.Left > 10*time.Minute {
		t.Fatalf("paus gav left=%v", s.Left)
	}
}

func TestDelShiftsActive(t *testing.T) {
	s := defaultState()
	for _, n := range []string{"a", "b", "c"} {
		s.apply(Req{Cmd: "add", S: n})
	}
	s.apply(Req{Cmd: "active", I: 2})
	s.apply(Req{Cmd: "del", I: 0})
	if s.Active != 1 || s.Tasks[s.Active].Title != "c" {
		t.Fatalf("active=%d", s.Active)
	}
}
