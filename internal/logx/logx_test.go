package logx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTagsMaskingAndFile(t *testing.T) {
	dir := t.TempDir()
	fixed := time.Date(2026, 10, 13, 9, 30, 0, 0, time.Local)
	l, err := newLogger(dir, 7, func() time.Time { return fixed })
	if err != nil {
		t.Fatal(err)
	}

	l.AddSecret("node-password")
	l.AddSecret("abc") // too short to mask safely
	l.Printf(App, "已加载配置 %s", "直连.json")
	fmt.Fprint(l.Writer(Task), "dial with node-password failed\nsecond line\r\n")
	l.Printf(App, "abc stays visible")
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(filepath.Join(dir, "local-20261013.log"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range []string{
		"2026-10-13 09:30:00 [程序] 已加载配置 直连.json",
		"[任务] dial with *** failed",
		"[任务] second line",
		"[程序] abc stays visible",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("log file lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "node-password") {
		t.Fatal("secrets must never reach the file")
	}
}

func TestRotationAndRetention(t *testing.T) {
	dir := t.TempDir()
	for _, day := range []string{"20261001", "20261005", "20261012", "notadate"} {
		os.WriteFile(filepath.Join(dir, "local-"+day+".log"), []byte("x\n"), 0o644)
	}
	os.WriteFile(filepath.Join(dir, "other.log"), []byte("x\n"), 0o644)

	now := time.Date(2026, 10, 13, 23, 59, 0, 0, time.Local)
	l, err := newLogger(dir, 7, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	l.Printf(App, "before midnight")
	now = now.Add(2 * time.Minute)
	l.Printf(App, "after midnight")
	l.Close()

	for name, want := range map[string]bool{
		"local-20261001.log": false, // older than 7 days
		"local-20261005.log": false,
		"local-20261012.log": true,
		"local-notadate.log": true, // not ours to judge
		"other.log":          true,
		"local-20261013.log": true,
		"local-20261014.log": true,
	} {
		_, err := os.Stat(filepath.Join(dir, name))
		if exists := err == nil; exists != want {
			t.Errorf("%s exists=%v, want %v", name, exists, want)
		}
	}
}

func TestHistoryAndSubscribers(t *testing.T) {
	l, err := New("", 0)
	if err != nil {
		t.Fatal(err)
	}
	ch, cancel := l.Subscribe()
	for i := 0; i < historySize+20; i++ {
		l.Printf(App, "line %d", i)
	}
	recent := l.Recent(3)
	if len(recent) != 3 || recent[2].Text != fmt.Sprintf("line %d", historySize+19) {
		t.Fatalf("recent %+v", recent)
	}
	for i, line := range recent {
		if want := uint64(historySize + 18 + i); line.Seq != want {
			t.Fatalf("recent[%d].Seq = %d, want %d", i, line.Seq, want)
		}
	}
	if got := len(l.Recent(0)); got != historySize {
		t.Fatalf("history keeps %d lines, want %d", got, historySize)
	}
	first := <-ch
	if first.Text != "line 0" || first.Tag != App || first.Seq != 1 {
		t.Fatalf("first event %+v", first)
	}
	cancel()
	cancel() // idempotent
	l.Printf(App, "after cancel")
	l.Close()
}
