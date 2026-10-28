package data

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeServer struct {
	*httptest.Server
	hits     atomic.Int32
	svgMain  bool // отдавать ли SVG по основному адресу
	fallback string
	fail     atomic.Bool   // отвечать 500 на всё
	hang     chan struct{} // не nil — запросы висят, пока канал не закрыт
}

func newServer(t *testing.T) *fakeServer {
	t.Helper()
	fs := &fakeServer{svgMain: true, hang: make(chan struct{})}
	close(fs.hang) // по умолчанию не висим; hangUntilCleanup включает
	files := map[string]string{
		"/pve/maps":            "testdata/pve_maps.json",
		"/pve/maps_ru":         "testdata/pve_maps_ru.json",
		"/maps.json":           "testdata/maps.json",
		"/svg/Interchange.svg": "testdata/Interchange.svg",
		"/fb/Interchange.svg":  "testdata/Interchange.svg",
	}
	fs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.hits.Add(1)
		<-fs.hangCh()
		if fs.fail.Load() {
			http.Error(w, "fail", http.StatusInternalServerError)
			return
		}
		if r.URL.Path == "/svg/Interchange.svg" && !fs.svgMain {
			http.NotFound(w, r)
			return
		}
		f, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, f)
	}))
	t.Cleanup(fs.Close)
	return fs
}

var hangMu sync.Mutex

func (fs *fakeServer) hangCh() chan struct{} {
	hangMu.Lock()
	defer hangMu.Unlock()
	return fs.hang
}

// hangUntilCleanup: «висящая» сеть (прокси без выхода наружу) до вызова release
// или конца теста.
func (fs *fakeServer) hangUntilCleanup(t *testing.T) (release func()) {
	ch := make(chan struct{})
	hangMu.Lock()
	fs.hang = ch
	hangMu.Unlock()
	var once sync.Once
	release = func() { once.Do(func() { close(ch) }) }
	t.Cleanup(release) // раньше fs.Close: Close ждёт активные запросы
	return release
}

// waitFor ждёт условие до 2 с.
func waitFor(cond func() bool) bool {
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if cond() {
			return true
		}
	}
	return cond()
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newStore(t *testing.T, fs *fakeServer, c *clock, dir string) *Store {
	t.Helper()
	s := NewStore(dir, fs.Client(), c.now)
	s.JSONBase = fs.URL
	s.MapsJSONURL = fs.URL + "/maps.json"
	s.SVGBase = fs.URL + "/svg"
	s.SVGFallback = fs.URL + "/fb"
	return s
}

// Файлы кэша получают «текущее» время часов, чтобы проверять свежесть.
func touchCache(t *testing.T, dir string, at time.Time) {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() {
			os.Chtimes(filepath.Join(dir, e.Name()), at, at)
		}
	}
}

func loaded(t *testing.T) (*Store, *fakeServer, *clock, string) {
	t.Helper()
	fs := newServer(t)
	c := &clock{t: time.Now()}
	dir := t.TempDir()
	s := newStore(t, fs, c, dir)
	if err := s.Load("pve", false); err != nil {
		t.Fatal(err)
	}
	touchCache(t, dir, c.t)
	return s, fs, c, dir
}

func TestLoadAndLookup(t *testing.T) {
	s, _, _, _ := loaded(t)
	m, ok := s.MapByScene("maps/shopping_mall.bundle")
	if !ok {
		t.Fatal("Развязка не найдена по scenePath")
	}
	if m.Name != "Развязка" || m.NormalizedName != "interchange" {
		t.Errorf("Name/NormalizedName = %q %q", m.Name, m.NormalizedName)
	}
	byName := map[string]Extract{}
	for _, e := range m.Extracts {
		byName[e.Name] = e
	}
	if byName["Безопасная комната"].Faction != "pmc" {
		t.Errorf("Безопасная комната: %+v", byName["Безопасная комната"])
	}
	if byName["Выход на железной дороге"].Faction != "scav" {
		t.Errorf("Выход на железной дороге: %+v", byName["Выход на железной дороге"])
	}
	if !byName["А-Выход у ТЭЦ"].Paid {
		t.Errorf("А-Выход у ТЭЦ не платный: %+v", byName["А-Выход у ТЭЦ"])
	}
	if len(m.Transits) != 1 || m.Transits[0].Name != "Переход на Таможню" {
		t.Errorf("Transits = %+v", m.Transits)
	}
}

// В реальных данных switch бывает bool, а бывает строкой — id рубильника (Таможня).
func TestSwitchFlag(t *testing.T) {
	cases := map[string]bool{`false`: false, `true`: true, `"5c1d0dc586f7744baf2e7b79"`: true, `""`: false, `null`: false, ``: false}
	for raw, want := range cases {
		if got := switchFlag([]byte(raw)); got != want {
			t.Errorf("switchFlag(%s) = %v, want %v", raw, got, want)
		}
	}
}

func TestMapDef(t *testing.T) {
	s, _, _, _ := loaded(t)
	d, ok := s.MapDef("interchange")
	if !ok {
		t.Fatal("нет MapDef Развязки")
	}
	if d.CoordinateRotation != 180 || d.Layers[0].SVGLayer != "First_Floor" {
		t.Errorf("rotation/layer = %v %q", d.CoordinateRotation, d.Layers[0].SVGLayer)
	}
	if b := d.Layers[0].Extents[0].Bounds[0]; b != [2][2]float64{{120, 218}, {-222, -327}} {
		t.Errorf("bounds = %v", b)
	}
	if f, ok := s.MapDef("night-factory"); !ok || f.Key != "factory" {
		t.Errorf("night-factory → %q %v", f.Key, ok)
	}
	if _, ok := s.MapDef("the-lab"); ok {
		t.Error("the-lab не должна поддерживаться")
	}
}

func TestLoadUsesCacheWithin24h(t *testing.T) {
	s, fs, c, _ := loaded(t)
	before := fs.hits.Load()
	c.t = c.t.Add(time.Hour)
	if err := s.Load("pve", false); err != nil {
		t.Fatal(err)
	}
	if n := fs.hits.Load() - before; n != 0 {
		t.Errorf("через час: %d запросов, want 0", n)
	}
	c.t = c.t.Add(24 * time.Hour)
	if err := s.Load("pve", false); err != nil {
		t.Fatal(err)
	}
	if fs.hits.Load() == before {
		t.Error("через 25 ч данные не обновлялись")
	}
}

func TestLoadOfflineWithCache(t *testing.T) {
	s, fs, c, _ := loaded(t)
	fs.Close()
	c.t = c.t.Add(25 * time.Hour)
	if err := s.Load("pve", false); err != nil {
		t.Fatalf("без сети с кэшем: %v", err)
	}
	if _, ok := s.MapByScene("maps/shopping_mall.bundle"); !ok {
		t.Error("данные пропали")
	}
}

func TestLoadOfflineNoCache(t *testing.T) {
	fs := newServer(t)
	c := &clock{t: time.Now()}
	s := newStore(t, fs, c, t.TempDir())
	fs.Close()
	if err := s.Load("pve", false); !errors.Is(err, ErrNoData) {
		t.Errorf("err = %v, want ErrNoData", err)
	}
}

func TestSVGFallbackAndCache(t *testing.T) {
	s, fs, _, _ := loaded(t)
	fs.svgMain = false
	d, _ := s.MapDef("interchange")
	svg, err := s.SVG(d)
	if err != nil || len(svg) == 0 {
		t.Fatalf("SVG: %v", err)
	}
	before := fs.hits.Load()
	if _, err := s.SVG(d); err != nil {
		t.Fatal(err)
	}
	if n := fs.hits.Load() - before; n != 0 {
		t.Errorf("повторный SVG: %d запросов", n)
	}
}

func TestLoadCachedIgnoresAge(t *testing.T) {
	_, fs, c, dir := loaded(t)
	s := newStore(t, fs, c, dir) // новый запуск
	fs.Close()
	c.t = c.t.Add(30 * 24 * time.Hour)
	if err := s.LoadCached("pve"); err != nil {
		t.Fatalf("LoadCached на месячном кэше: %v", err)
	}
	if _, ok := s.MapByScene("maps/shopping_mall.bundle"); !ok {
		t.Error("данные из кэша не разобраны")
	}
	if !s.Stale("pve") {
		t.Error("месячный кэш не считается устаревшим")
	}
	if err := newStore(t, fs, c, t.TempDir()).LoadCached("pve"); !errors.Is(err, ErrNoData) {
		t.Errorf("без кэша: %v, want ErrNoData", err)
	}
}

func TestFetchReportsNetworkError(t *testing.T) {
	_, fs, c, dir := loaded(t)
	s := newStore(t, fs, c, dir)
	fs.fail.Store(true)
	if err := s.Fetch("pve"); err == nil {
		t.Error("Fetch при ошибке сервера вернул nil")
	}
	if err := s.LoadCached("pve"); err != nil {
		t.Errorf("кэш испорчен неудачным Fetch: %v", err)
	}
}

func TestSVGStaleServedAtOnceAndRefreshed(t *testing.T) {
	s, fs, c, dir := loaded(t)
	d, _ := s.MapDef("interchange")
	if _, err := s.SVG(d); err != nil {
		t.Fatal(err)
	}
	c.t = c.t.Add(25 * time.Hour)
	release := fs.hangUntilCleanup(t)
	before := fs.hits.Load()
	done := make(chan struct{})
	go func() { s.SVG(d); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("устаревший SVG ждёт сеть вместо отдачи из кэша")
	}
	if !waitFor(func() bool { return fs.hits.Load() > before }) {
		t.Error("устаревший SVG не обновляется в фоне")
	}
	// дождаться конца фоновой закачки, иначе она пишет в уже удалённую папку теста
	release()
	p := filepath.Join(dir, "svg", "Interchange.svg")
	if !waitFor(func() bool { _, busy := s.svgRefresh.Load(p); return !busy }) {
		t.Error("фоновое обновление SVG не завершилось")
	}
}

func TestRefreshSVG(t *testing.T) {
	s, fs, c, _ := loaded(t)
	d, _ := s.MapDef("interchange")
	if _, err := s.SVG(d); err != nil {
		t.Fatal(err)
	}
	before := fs.hits.Load()
	c.t = c.t.Add(time.Minute)
	if err := s.RefreshSVG(d); err != nil {
		t.Fatalf("RefreshSVG: %v", err)
	}
	if fs.hits.Load() == before {
		t.Error("RefreshSVG не перекачал свежий SVG")
	}
	fs.fail.Store(true)
	if err := s.RefreshSVG(d); err == nil {
		t.Error("RefreshSVG без сети вернул nil")
	}
	if svg, err := s.SVG(d); err != nil || svg == "" {
		t.Errorf("после неудачного обновления SVG пропал: %v", err)
	}
}
