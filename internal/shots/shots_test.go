package shots

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Vadimushka/tarkov-map/internal/mapgeo"
)

const real0 = "2026-10-28[21-06]_-312.74, 23.88, -111.95_-0.04637, 0.85289, 0.07698, 0.51430_9.75 (0).png"

func TestParseName(t *testing.T) {
	s, ok := ParseName(real0)
	if !ok {
		t.Fatal("ParseName вернул false")
	}
	if s.Name != real0 || s.Date != "2026-10-28" || s.Time != "21:06" {
		t.Errorf("Name/Date/Time = %q %q %q", s.Name, s.Date, s.Time)
	}
	if s.Pos != (mapgeo.Vec3{X: -312.74, Y: 23.88, Z: -111.95}) {
		t.Errorf("Pos = %v", s.Pos)
	}
	if s.Q != [4]float64{-0.04637, 0.85289, 0.07698, 0.51430} {
		t.Errorf("Q = %v", s.Q)
	}
}

func TestParseNameDuplicateKey(t *testing.T) {
	a, okA := ParseName("2026-10-28[21-15]_-304.45, 24.18, 146.26_0.01327, 0.79807, -0.01759, 0.60216_10.81 (0).png")
	b, okB := ParseName("2026-10-28[21-15]_-304.45, 24.18, 146.26_0.01327, 0.79807, -0.01759, 0.60216_10.81 (1).png")
	if !okA || !okB {
		t.Fatal("не разобрались")
	}
	if a.Key() != b.Key() {
		t.Errorf("ключи различаются: %q vs %q", a.Key(), b.Key())
	}
	c, _ := ParseName(real0)
	if a.Key() == c.Key() {
		t.Error("разные скрины дали одинаковый ключ")
	}
}

func TestParseNameInvalid(t *testing.T) {
	for _, n := range []string{"photo.png", "2026-10-28[21-06]_abc.png", real0[:len(real0)-4]} {
		if _, ok := ParseName(n); ok {
			t.Errorf("ParseName(%q) = true", n)
		}
	}
}

func startWatcher(t *testing.T) (string, chan string) {
	t.Helper()
	dir := t.TempDir()
	got := make(chan string, 10)
	w, err := NewWatcher(dir, func(name string, at time.Time) { got <- name })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	return dir, got
}

func TestWatcherReportsPng(t *testing.T) {
	dir, got := startWatcher(t)
	if err := os.WriteFile(filepath.Join(dir, real0), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case n := <-got:
		if n != real0 {
			t.Errorf("имя = %q", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("нет события за 2 c")
	}
}

func TestWatcherIgnoresOther(t *testing.T) {
	dir, got := startWatcher(t)
	if err := os.WriteFile(filepath.Join(dir, "x.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case n := <-got:
		t.Errorf("лишнее событие: %q", n)
	case <-time.After(time.Second):
	}
}

func TestWatcherCreateThenDelete(t *testing.T) {
	dir, got := startWatcher(t)
	p := filepath.Join(dir, real0)
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	select {
	case n := <-got:
		if n != real0 {
			t.Errorf("имя = %q", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("скрин, удалённый сразу после создания, потерян")
	}
}
