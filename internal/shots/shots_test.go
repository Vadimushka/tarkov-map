package shots

import (
	"fmt"
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

// Игра создаёт папку Screenshots при первом скрине: до этого следить не за чем,
// но и ошибкой это не считается — скрины подхватятся, как только папка появится.
func TestWatcherWaitsForDir(t *testing.T) {
	old := retryEvery
	retryEvery = 20 * time.Millisecond
	t.Cleanup(func() { retryEvery = old })
	dir := filepath.Join(t.TempDir(), "Screenshots")
	got := make(chan string, 10)
	w, err := NewWatcher(dir, func(name string, at time.Time) { got <- name })
	if err != nil {
		t.Fatalf("NewWatcher без папки: %v", err)
	}
	t.Cleanup(func() { w.Close() })
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for i := 0; ; i++ {
		// папку подхватывают по таймеру — пишем скрины, пока не заметят
		name := fmt.Sprintf("2026-10-30[21-00]_%d.png", i)
		os.WriteFile(filepath.Join(dir, name), nil, 0o644)
		select {
		case <-got:
			return
		case <-time.After(100 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			t.Fatal("скрин в появившейся папке не замечен")
		}
	}
}

// Игра создаёт папку вместе с первым скрином: события Create для него уже не будет,
// но скрин всё равно должен дойти.
func TestWatcherFirstShotInNewDir(t *testing.T) {
	old := retryEvery
	retryEvery = 20 * time.Millisecond
	t.Cleanup(func() { retryEvery = old })
	dir := filepath.Join(t.TempDir(), "Screenshots")
	got := make(chan string, 10)
	w, err := NewWatcher(dir, func(name string, at time.Time) { got <- name })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	os.Mkdir(dir, 0o755)
	os.WriteFile(filepath.Join(dir, real0), nil, 0o644)
	select {
	case name := <-got:
		if name != real0 {
			t.Errorf("скрин = %q", name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("первый скрин в новой папке потерян")
	}
}

// Папку скриншотов удалили и создали заново (чистка, переустановка игры): слежение
// возобновляется без перезапуска программы.
func TestWatcherDirRecreated(t *testing.T) {
	old := retryEvery
	retryEvery = 20 * time.Millisecond
	t.Cleanup(func() { retryEvery = old })
	dir := filepath.Join(t.TempDir(), "Screenshots")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	got := make(chan string, 10)
	w, err := NewWatcher(dir, func(name string, at time.Time) { got <- name })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	// Windows держит удалённую папку, пока слежение не отпустит её
	deadline := time.Now().Add(3 * time.Second)
	for err := os.Mkdir(dir, 0o755); err != nil; err = os.Mkdir(dir, 0o755) {
		if time.Now().After(deadline) {
			t.Fatalf("папку не создать заново: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	for i := 0; ; i++ {
		name := fmt.Sprintf("2026-10-30[22-00]_%d.png", i)
		os.WriteFile(filepath.Join(dir, name), nil, 0o644)
		select {
		case <-got:
			return
		case <-time.After(100 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			t.Fatal("скрин в пересозданной папке не замечен")
		}
	}
}
