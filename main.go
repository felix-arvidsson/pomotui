package main

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

const usage = `pomo – pomofocus i terminalen

  pomo                 öppna TUI (startar daemonen vid behov)
  pomo status          en rad
  pomo waybar          JSON-ström för waybars custom-modul
  pomo toggle|skip|reset
  pomo set <nyckel> <värde>   nycklar: pomodoro short long interval autobreaks autopomos
  pomo quit            stoppa daemonen
  pomo daemon          kör daemonen i förgrunden`

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		runTUI()
		return
	}
	var st *State
	var err error
	switch cmd := args[0]; cmd {
	case "daemon":
		runDaemon()
	case "waybar":
		waybar()
	case "status":
		st, err = call(Req{Cmd: "status"}, false)
		if err == nil {
			fmt.Println(st.line())
		}
	case "toggle", "skip", "reset":
		_, err = call(Req{Cmd: cmd}, true)
	case "quit":
		_, err = call(Req{Cmd: cmd}, false)
	case "set":
		if len(args) != 3 || !slices.Contains(settingKeys, args[1]) {
			die(usage)
		}
		n, perr := strconv.Atoi(args[2])
		if perr != nil {
			b, berr := strconv.ParseBool(args[2])
			if berr != nil {
				die("pomo: ogiltigt värde " + args[2])
			}
			n = map[bool]int{true: 1}[b]
		}
		_, err = call(Req{Cmd: "set", S: args[1], N: n}, true)
	default:
		die(usage)
	}
	if err != nil {
		die("pomo: " + err.Error())
	}
}

func die(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}

func clock(d time.Duration) string {
	sec := int((d + time.Second - 1) / time.Second) // avrunda uppåt: 25:00 visas tills första sekunden gått
	return fmt.Sprintf("%02d:%02d", sec/60, sec%60)
}

func (s *State) line() string {
	icon := [...]string{"🍅", "☕", "🌴"}[s.Mode]
	if !s.Running {
		icon = "⏸"
	}
	return fmt.Sprintf("%s %s #%d", icon, clock(s.Left), s.Round+1)
}

// waybar skriver en JSON-rad varje gång något ändras; modulen körs utan interval.
func waybar() {
	last := ""
	for {
		out := map[string]string{"text": "🍅", "class": "off", "tooltip": "pomo: daemonen kör inte\nklicka för att starta en pomodoro"}
		if st, err := call(Req{Cmd: "status"}, false); err == nil {
			out = map[string]string{"text": st.line(), "class": "paused", "tooltip": st.tooltip()}
			if st.Running {
				out["class"] = [...]string{"pomodoro", "short", "long"}[st.Mode]
			}
		}
		b, _ := json.Marshal(out)
		if string(b) != last {
			fmt.Println(string(b))
			last = string(b)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func (s *State) tooltip() string {
	t := []string{[...]string{"Pomodoro", "Kort paus", "Lång paus"}[s.Mode] + fmt.Sprintf(" #%d", s.Round+1)}
	if s.valid(s.Active) {
		t = append(t, "Uppgift: "+html.EscapeString(s.Tasks[s.Active].Title)) // tooltip är pango-markup
	}
	if sum := s.summary(); sum != "" {
		t = append(t, sum)
	}
	return strings.Join(append(t, "", "vänster: start/paus · mitten: skippa · höger: öppna"), "\n")
}
