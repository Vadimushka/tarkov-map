package core

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Vadimushka/tarkov-map/internal/data"
	"github.com/Vadimushka/tarkov-map/internal/gamelog"
	"github.com/Vadimushka/tarkov-map/internal/mapgeo"
)

var ic = mapgeo.MapDef{
	Key: "interchange", Transform: []float64{0.265, 150.6, 0.265, 134.6}, CoordinateRotation: 180,
	Bounds: [2][2]float64{{598, -442}, {-433, 426}}, SVGLayer: "Ground_Level",
	Layers: []mapgeo.Layer{{Name: "2nd Floor", SVGLayer: "First_Floor", Extents: []mapgeo.Extent{{Height: [2]float64{25, 34}, Bounds: [][2][2]float64{{{120, 218}, {-222, -327}}}}}}},
}

type fakeSrc struct{}

func (fakeSrc) MapByScene(scene string) (data.GameMap, bool) {
	switch scene {
	case "maps/shopping_mall.bundle":
		return data.GameMap{NormalizedName: "interchange", Name: "Развязка", ScenePath: scene,
			Extracts: []data.Extract{{Name: "Безопасная комната", Faction: "pmc", Pos: mapgeo.Vec3{X: -48, Y: 23, Z: 44}}},
			Transits: []data.Transit{{Name: "Переход на Таможню", Pos: mapgeo.Vec3{X: 280, Y: 20, Z: 435}}}}, true
	case "maps/laboratory_preset.bundle":
		return data.GameMap{NormalizedName: "the-lab", Name: "Лаборатория", ScenePath: scene}, true
	case "maps/sandbox_start_preset.bundle": // обучающий Эпицентр: ни выходов, ни переходов
		return data.GameMap{NormalizedName: "ground-zero", Name: "Эпицентр", ScenePath: scene}, true
	}
	return data.GameMap{}, false
}

func (fakeSrc) MapDef(n string) (mapgeo.MapDef, bool) {
	if n == "interchange" || n == "ground-zero" {
		return ic, true
	}
	return mapgeo.MapDef{}, false
}

// Фронтенд перебирает Markers: null вместо [] ломал отрисовку карты без меток.
func TestViewWithoutMarkers(t *testing.T) {
	c := New(fakeSrc{})
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvMap, At: at(19, 47, 26), Bundle: "maps/sandbox_start_preset.bundle"})
	b, err := json.Marshal(c.View())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"Markers":[]`) || strings.Contains(string(b), `"Layers":null`) {
		t.Errorf("View без меток: %s", b)
	}
}

const (
	shot1    = "2026-10-28[21-06]_-312.74, 23.88, -111.95_-0.04637, 0.85289, 0.07698, 0.51430_9.75 (0).png"
	shot1dup = "2026-10-28[21-06]_-312.74, 23.88, -111.95_-0.04637, 0.85289, 0.07698, 0.51430_9.75 (1).png"
	shot2    = "2026-10-28[21-36]_-51.12, 22.88, 47.90_0.07448, 0.32045, -0.02528, 0.94399_13.27 (0).png"
	shotLook = "2026-10-28[21-40]_0.00, 22.00, 0.00_0.00000, 0.70711, 0.00000, 0.70711_13.67 (0).png"
)

func at(h, m, s int) time.Time { return time.Date(2026, 10, 28, h, m, s, 0, time.Local) }

func inRaid() *Core {
	c := New(fakeSrc{})
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvMode, At: at(21, 2, 54), Mode: "pve"})
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvMap, At: at(21, 4, 44), Bundle: "maps/shopping_mall.bundle"})
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvRaidStart, At: at(21, 6, 12)})
	return c
}

func shotMarkers(v MapView) []Marker {
	var out []Marker
	for _, m := range v.Markers {
		if m.Kind == "shot" {
			out = append(out, m)
		}
	}
	return out
}

func TestRaidFlow(t *testing.T) {
	c := inRaid()
	if !c.OnShot(shot1, at(21, 6, 30)) {
		t.Fatal("OnShot = false")
	}
	v := c.View()
	sm := shotMarkers(v)
	if len(sm) != 1 || sm[0].Layer != "Ground_Level" {
		t.Fatalf("shot-маркеры = %+v", sm)
	}
	px, py := mapgeo.Project(ic, -312.74, -111.95)
	if sm[0].PX != px || sm[0].PY != py {
		t.Errorf("PX/PY = %v,%v want %v,%v", sm[0].PX, sm[0].PY, px, py)
	}
	s := c.Snapshot()
	if s.Mode != "pve" || s.MapID != "interchange" || s.MapName != "Развязка" || !s.Supported || s.LastShot != "21:06" {
		t.Errorf("Snapshot = %+v", s)
	}
	if len(v.Markers) != 3 { // выход, переход, точка
		t.Errorf("маркеров %d, want 3", len(v.Markers))
	}
}

func TestDuplicateShot(t *testing.T) {
	c := inRaid()
	c.OnShot(shot1, at(21, 6, 30))
	if c.OnShot(shot1dup, at(21, 6, 31)) {
		t.Error("дубль принят")
	}
	if n := len(shotMarkers(c.View())); n != 1 {
		t.Errorf("точек %d", n)
	}
}

func TestShotBeforeRaidIgnored(t *testing.T) {
	c := New(fakeSrc{})
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvMap, At: at(21, 4, 44), Bundle: "maps/shopping_mall.bundle"})
	if c.OnShot(shot1, at(21, 5, 0)) {
		t.Error("скрин до GameStarted принят")
	}
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvRaidStart, At: at(21, 6, 12)})
	if c.OnShot(shot1, at(21, 6, 0)) {
		t.Error("скрин раньше начала рейда принят")
	}
}

func TestNewMapClearsRoute(t *testing.T) {
	c := inRaid()
	c.OnShot(shot1, at(21, 6, 30))
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvMap, At: at(21, 50, 0), Bundle: "maps/shopping_mall.bundle"})
	if n := len(shotMarkers(c.View())); n != 0 {
		t.Errorf("после новой карты точек %d", n)
	}
}

func TestUnsupportedMap(t *testing.T) {
	c := New(fakeSrc{})
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvMap, At: at(21, 4, 44), Bundle: "maps/laboratory_preset.bundle"})
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvRaidStart, At: at(21, 6, 12)})
	s := c.Snapshot()
	if s.Supported || s.Message != "Карта «Лаборатория» пока не поддерживается" {
		t.Errorf("Snapshot = %+v", s)
	}
	if c.OnShot(shot1, at(21, 7, 0)) {
		t.Error("скрин на неподдержанной карте принят")
	}
	if v := c.View(); v.Supported || v.Name != "Лаборатория" {
		t.Errorf("View = %+v", v)
	}
}

func TestShotDirection(t *testing.T) {
	c := inRaid()
	c.OnShot(shotLook, at(21, 40, 0))
	m := shotMarkers(c.View())[0]
	if math.Abs(m.DirX+1) > 1e-3 || math.Abs(m.DirY) > 1e-3 {
		t.Errorf("Dir = (%v, %v), want (-1, 0)", m.DirX, m.DirY)
	}
}

func TestRestoreMidRaid(t *testing.T) {
	c := inRaid() // реплей текущей сессии логов
	old := "2026-10-28[20-40]_100.00, 22.00, 100.00_0.00000, 0.00000, 0.00000, 1.00000_8.00 (0).png"
	c.OnShot(old, at(20, 40, 0)) // скрин прошлого рейда — ModTime раньше GameStarted
	c.OnShot(shot1, at(21, 6, 30))
	c.OnShot(shot2, at(21, 36, 5))
	sm := shotMarkers(c.View())
	if len(sm) != 2 || sm[0].Time != "21:06" || sm[1].Time != "21:36" {
		t.Errorf("маршрут = %+v", sm)
	}
}

// Скрин из убежища или меню после рейда (в имени есть координаты) не попадает на прошлую карту.
func TestShotAfterRaidEndIgnored(t *testing.T) {
	c := inRaid()
	c.OnShot(shot1, at(21, 6, 30))
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvRaidEnd, At: at(21, 25, 1)})
	hideout := "2026-10-28[21-33]_2.08, 41.65, 0.48_0.05183, -0.64508, 0.04393, 0.76109_6.76 (0).png"
	if c.OnShot(hideout, at(21, 33, 8)) {
		t.Error("скрин из убежища принят")
	}
	if sm := shotMarkers(c.View()); len(sm) != 1 || sm[0].Time != "21:06" {
		t.Errorf("после конца рейда маршрут = %+v, want прежний", sm)
	}
}

// Перезапуск приложения после рейда: реплей логов уже видит конец рейда,
// но скрины самого рейда (по времени файла) должны восстановиться.
func TestRestoreAfterRaidEnd(t *testing.T) {
	c := inRaid()
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvRaidEnd, At: at(21, 25, 1)})
	c.OnShot(shot1, at(21, 6, 30))
	if n := len(shotMarkers(c.View())); n != 1 {
		t.Errorf("скринов рейда после реплея %d, want 1", n)
	}
}

// Первый скрин через пару секунд после спавна приходит раньше, чем прочитана строка
// GameStarted (лог читается раз в секунду): его надо придержать, а не выбросить.
func TestEarlyShotHeldUntilRaidStart(t *testing.T) {
	c := New(fakeSrc{})
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvMap, At: at(21, 4, 44), Bundle: "maps/shopping_mall.bundle"})
	c.OnShot(shot1, at(21, 6, 15)) // файл появился, строка о старте ещё не прочитана
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvRaidStart, At: at(21, 6, 12)})
	if sm := shotMarkers(c.View()); len(sm) != 1 || sm[0].Time != "21:06" {
		t.Errorf("ранний скрин потерян: %+v", sm)
	}
}

// Придержанные скрины не переживают смену карты.
func TestHeldShotsDroppedOnNewMap(t *testing.T) {
	c := New(fakeSrc{})
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvMap, At: at(21, 4, 44), Bundle: "maps/shopping_mall.bundle"})
	c.OnShot(shot1, at(21, 6, 15))
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvMap, At: at(21, 30, 0), Bundle: "maps/shopping_mall.bundle"})
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvRaidStart, At: at(21, 6, 12)})
	if n := len(shotMarkers(c.View())); n != 0 {
		t.Errorf("скрин прошлой загрузки карты попал в маршрут: %d", n)
	}
}
