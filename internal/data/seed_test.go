package data

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

// seedFS — снимок из testdata со временем at, пути как в кэше.
func seedFS(t *testing.T, at time.Time) fstest.MapFS {
	t.Helper()
	m := fstest.MapFS{}
	for name, src := range map[string]string{
		"maps.json":           "testdata/maps.json",
		"pve_maps.json":       "testdata/pve_maps.json",
		"pve_maps_ru.json":    "testdata/pve_maps_ru.json",
		"pve_items_ru.json":   "testdata/pve_items_ru.json",
		"svg/Interchange.svg": "testdata/Interchange.svg",
	} {
		b, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		m[name] = &fstest.MapFile{Data: b, ModTime: at}
	}
	return m
}

func modTime(t *testing.T, p string) time.Time {
	t.Helper()
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return st.ModTime()
}

func TestSeedFillsEmptyCacheWithoutNetwork(t *testing.T) {
	fs := newServer(t)
	fs.fail.Store(true)
	c := &clock{t: time.Now()}
	dir := t.TempDir()
	s := newStore(t, fs, c, dir)
	at := c.t.Add(-48 * time.Hour).Truncate(time.Second)
	s.Seed = seedFS(t, at)
	if err := s.LoadCached("pve"); err != nil {
		t.Fatalf("LoadCached на снимке: %v", err)
	}
	if _, ok := s.MapByScene("maps/shopping_mall.bundle"); !ok {
		t.Error("карта из снимка не разобрана")
	}
	if _, ok := s.ItemName("i_flash"); !ok {
		t.Error("названия предметов из снимка не разобраны")
	}
	if got := modTime(t, filepath.Join(dir, "pve_maps.json")); !got.Equal(at) {
		t.Errorf("время файла = %v, want время снимка %v", got, at)
	}
	if !s.Stale("pve") {
		t.Error("двухдневный снимок не считается устаревшим — не обновится из сети")
	}
	if fs.hits.Load() != 0 {
		t.Errorf("LoadCached ходил в сеть: %d запросов", fs.hits.Load())
	}
}

func TestSeedReplacesOlderCache(t *testing.T) {
	_, fs, c, dir := loaded(t)
	s := newStore(t, fs, c, dir)
	at := c.t.Add(time.Hour).Truncate(time.Second)
	s.Seed = seedFS(t, at)
	if err := s.LoadCached("pve"); err != nil {
		t.Fatal(err)
	}
	if got := modTime(t, filepath.Join(dir, "pve_maps.json")); !got.Equal(at) {
		t.Errorf("кэш старше снимка не заменён: время %v, want %v", got, at)
	}
}

func TestSeedKeepsNewerCache(t *testing.T) {
	_, fs, c, dir := loaded(t)
	s := newStore(t, fs, c, dir)
	s.Seed = seedFS(t, c.t.Add(-48*time.Hour))
	if err := s.LoadCached("pve"); err != nil {
		t.Fatal(err)
	}
	if got := modTime(t, filepath.Join(dir, "pve_maps.json")); !got.Equal(c.t) {
		t.Errorf("свежий кэш откатился к снимку: время %v, want %v", got, c.t)
	}
}

func TestSeedSVGWithoutNetwork(t *testing.T) {
	fs := newServer(t)
	fs.fail.Store(true)
	c := &clock{t: time.Now()}
	s := newStore(t, fs, c, t.TempDir())
	s.Seed = seedFS(t, c.t.Truncate(time.Second))
	if err := s.LoadCached("pve"); err != nil {
		t.Fatal(err)
	}
	d, _ := s.MapDef("interchange")
	svg, err := s.SVG(d)
	if err != nil || len(svg) == 0 {
		t.Fatalf("SVG из снимка: %v", err)
	}
	if fs.hits.Load() != 0 {
		t.Errorf("SVG из свежего снимка ходил в сеть: %d запросов", fs.hits.Load())
	}
}

func TestFetchSkipsFreshFiles(t *testing.T) {
	_, fs, c, dir := loaded(t)
	s := newStore(t, fs, c, dir)
	old := c.t.Add(-25 * time.Hour)
	os.Chtimes(filepath.Join(dir, "pve_items_ru.json"), old, old)
	before := fs.hits.Load()
	if err := s.Fetch("pve", false); err != nil {
		t.Fatal(err)
	}
	if n := fs.hits.Load() - before; n != 1 {
		t.Errorf("фоновое обновление: %d запросов, want 1 (только устаревшие названия)", n)
	}
	before = fs.hits.Load()
	if err := s.Fetch("pve", true); err != nil {
		t.Fatal(err)
	}
	if n := fs.hits.Load() - before; n != 4 {
		t.Errorf("кнопка «Обновить данные»: %d запросов, want 4 (все файлы)", n)
	}
}
