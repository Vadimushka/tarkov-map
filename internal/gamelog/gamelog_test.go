package gamelog

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const (
	lMode  = "2026-10-28 21:02:54.762|1.2.5.0.48146|Info|application|Session mode: Pve"
	lReg   = "2026-10-28 21:02:24.793|1.2.5.0.48146|Info|application|Session mode: Regular"
	lMap   = "2026-10-28 21:04:44.152|1.2.5.0.48146|Info|application|scene preset path:maps/shopping_mall.bundle rcid:Shopping_Mall.ScenesPreset.asset"
	lStart = "2026-10-28 21:06:12.592|1.2.5.0.48146|Info|application|GameStarted:69.1(0) real:89.98(0) diff:20.88"
	lOther = "2026-10-28 21:04:44.152|1.2.5.0.48146|Debug|application|TRACE-NetworkGameMatching G"
)

func TestParseLine(t *testing.T) {
	if e, ok := ParseLine(lMode); !ok || e.Kind != EvMode || e.Mode != "pve" {
		t.Errorf("lMode → %+v %v", e, ok)
	}
	if e, ok := ParseLine(lReg); !ok || e.Kind != EvMode || e.Mode != "regular" {
		t.Errorf("lReg → %+v %v", e, ok)
	}
	if e, ok := ParseLine(lMap); !ok || e.Kind != EvMap || e.Bundle != "maps/shopping_mall.bundle" {
		t.Errorf("lMap → %+v %v", e, ok)
	}
	e, ok := ParseLine(lStart)
	want := time.Date(2026, 10, 28, 21, 6, 12, 592e6, time.Local)
	if !ok || e.Kind != EvRaidStart || !e.At.Equal(want) {
		t.Errorf("lStart → %+v %v", e, ok)
	}
	for _, l := range []string{lOther, "", "   at Foo()"} {
		if e, ok := ParseLine(l); ok {
			t.Errorf("ParseLine(%q) → %+v", l, e)
		}
	}
}

type sink struct{ evs []Event }

func (s *sink) add(e Event) { s.evs = append(s.evs, e) }

func (s *sink) take() []Event {
	r := s.evs
	s.evs = nil
	return r
}

func mkSession(t *testing.T, logs, name string, mod time.Time, content string) string {
	t.Helper()
	dir := filepath.Join(logs, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(dir, "2026.10.28_21-01-48_1.2.5.0.48146 application_000.log")
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dir, mod, mod); err != nil {
		t.Fatal(err)
	}
	return f
}

func appendTo(t *testing.T, f, s string) {
	t.Helper()
	h, err := os.OpenFile(f, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if _, err := h.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

func kinds(evs []Event) []Kind {
	var k []Kind
	for _, e := range evs {
		k = append(k, e.Kind)
	}
	return k
}

func TestTailerReadsSessionFromStart(t *testing.T) {
	logs := t.TempDir()
	mkSession(t, logs, "log_a", time.Now().Add(-time.Hour), lMode+"\n"+lOther+"\n"+lMap+"\n")
	s := &sink{}
	tl := NewTailer(logs, s.add)
	if err := tl.Poll(); err != nil {
		t.Fatal(err)
	}
	if k := kinds(s.take()); len(k) != 2 || k[0] != EvMode || k[1] != EvMap {
		t.Errorf("события = %v, want [EvMode EvMap]", k)
	}
}

func TestTailerPartialLine(t *testing.T) {
	logs := t.TempDir()
	f := mkSession(t, logs, "log_a", time.Now().Add(-time.Hour), lMap+"\n")
	s := &sink{}
	tl := NewTailer(logs, s.add)
	tl.Poll()
	s.take()
	appendTo(t, f, lStart)
	tl.Poll()
	if evs := s.take(); len(evs) != 0 {
		t.Fatalf("неполная строка потреблена: %v", kinds(evs))
	}
	appendTo(t, f, "\n")
	tl.Poll()
	if k := kinds(s.take()); len(k) != 1 || k[0] != EvRaidStart {
		t.Errorf("события = %v, want [EvRaidStart]", k)
	}
	tl.Poll()
	if evs := s.take(); len(evs) != 0 {
		t.Errorf("повтор: %v", kinds(evs))
	}
}

func TestTailerSwitchesSession(t *testing.T) {
	logs := t.TempDir()
	mkSession(t, logs, "log_a", time.Now().Add(-time.Hour), lMode+"\n")
	s := &sink{}
	tl := NewTailer(logs, s.add)
	tl.Poll()
	s.take()
	mkSession(t, logs, "log_b", time.Now(), lMap+"\n")
	tl.Poll()
	if k := kinds(s.take()); len(k) != 1 || k[0] != EvMap {
		t.Errorf("события = %v, want [EvMap]", k)
	}
}

func TestTailerNoLogsDir(t *testing.T) {
	s := &sink{}
	tl := NewTailer(filepath.Join(t.TempDir(), "нет"), s.add)
	if err := tl.Poll(); err != nil {
		t.Errorf("Poll = %v", err)
	}
	if len(s.evs) != 0 {
		t.Errorf("события: %v", kinds(s.evs))
	}
}
