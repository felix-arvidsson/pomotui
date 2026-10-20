package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

func sockPath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "pomo.sock")
}

type daemon struct {
	mu sync.Mutex
	st State
}

func runDaemon() {
	path := sockPath()
	if c, err := net.Dial("unix", path); err == nil {
		c.Close()
		fmt.Fprintln(os.Stderr, "pomo: daemon kör redan")
		os.Exit(1)
	}
	os.Remove(path) // gammal socket från en daemon som dog
	ln, err := net.Listen("unix", path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pomo:", err)
		os.Exit(1)
	}
	d := &daemon{st: loadState()}

	signal.Ignore(syscall.SIGHUP)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-sig; d.exit() }()
	d.check() // timer som gick ut medan daemonen var nere
	go func() {
		for range time.Tick(250 * time.Millisecond) {
			d.check()
		}
	}()

	for {
		c, err := ln.Accept()
		if err != nil {
			continue
		}
		go d.serve(c)
	}
}

func (d *daemon) exit() {
	d.mu.Lock()
	d.st.save()
	os.Remove(sockPath())
	os.Exit(0)
}

func (d *daemon) check() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.st.Running && !time.Now().Before(d.st.EndAt) {
		was := d.st.Mode
		d.st.finish()
		d.st.save()
		go alert(was)
	}
}

func (d *daemon) serve(c net.Conn) {
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Second))
	var r Req
	if err := json.NewDecoder(c).Decode(&r); err != nil {
		return
	}
	d.mu.Lock()
	if r.Cmd != "status" {
		d.st.apply(r)
		d.st.save()
	}
	snap := d.st
	snap.Left = d.st.left()
	b, _ := json.Marshal(snap)
	d.mu.Unlock()
	c.Write(append(b, '\n'))
	if r.Cmd == "quit" {
		d.exit()
	}
}

func alert(mode int) {
	msg := "Pomodoro klar – dags för paus!"
	if mode != modePomo {
		msg = "Pausen är slut – dags att fokusera!"
	}
	exec.Command("notify-send", "-a", "pomo", "🍅 pomo", msg).Run()
	exec.Command("paplay", "/usr/share/sounds/freedesktop/stereo/alarm-clock-elapsed.oga").Run()
}

// call skickar en request till daemonen och startar den om spawn är satt och den inte kör.
func call(r Req, spawn bool) (*State, error) {
	c, err := net.Dial("unix", sockPath())
	if err != nil {
		if !spawn {
			return nil, err
		}
		if c, err = spawnDaemon(); err != nil {
			return nil, err
		}
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Second))
	if err := json.NewEncoder(c).Encode(r); err != nil {
		return nil, err
	}
	var st State
	if err := json.NewDecoder(c).Decode(&st); err != nil {
		return nil, err
	}
	return &st, nil
}

func spawnDaemon() (net.Conn, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(exe, "daemon")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // egen session: överlever att terminalen stängs
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go cmd.Wait() // städa zombien om den dör medan klienten lever
	for range 40 {
		time.Sleep(50 * time.Millisecond)
		if c, err := net.Dial("unix", sockPath()); err == nil {
			return c, nil
		}
	}
	return nil, errors.New("daemonen startade inte")
}
