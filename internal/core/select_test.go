package core

import (
	"strings"
	"testing"

	"github.com/Vadimushka/tarkov-map/internal/gamelog"
)

// Ручной выбор карты меняет только то, что на экране: рейд (маршрут, скрины) живёт своей картой.

func TestSelectMapWithoutGame(t *testing.T) {
	c := New(fakeSrc{})
	if !strings.Contains(c.Snapshot().Message, "выберите карту") {
		t.Errorf("без карты из игры подсказать про ручной выбор: %q", c.Snapshot().Message)
	}
	c.SelectMap("maps/shopping_mall.bundle")
	v := c.View()
	if !v.Supported || v.MapID != "interchange" || len(v.Markers) != 2 { // выход и переход
		t.Fatalf("View = %+v", v)
	}
	if s := c.Snapshot(); !s.Manual || s.MapID != "interchange" || s.MapName != "Развязка" || !s.Supported || s.Message != "" {
		t.Errorf("Snapshot = %+v", s)
	}
	if m := c.Content(); m.MapID != "interchange" || len(m.Containers) != 1 || len(m.Places) != 2 {
		t.Errorf("Content: карта %q, контейнеров %d, мест %d", m.MapID, len(m.Containers), len(m.Places))
	}
}

func TestSelectOtherMapHidesRoute(t *testing.T) {
	c := inRaid() // Развязка
	if !c.OnShot(shot1, at(21, 6, 30)) {
		t.Fatal("OnShot = false")
	}
	c.SelectMap("maps/sandbox_start_preset.bundle")
	if v := c.View(); v.MapID != "ground-zero" || len(shotMarkers(v)) != 0 {
		t.Errorf("карта %q, точек маршрута %d — маршрут Развязки не рисуется на другой карте", v.MapID, len(shotMarkers(v)))
	}
	if !c.OnShot(shot2, at(21, 7, 0)) {
		t.Error("скрины рейда принимаются и пока на экране другая карта")
	}
	c.SelectMap("") // снова как в игре
	if v := c.View(); v.MapID != "interchange" || len(shotMarkers(v)) != 2 {
		t.Errorf("после возврата: карта %q, точек %d, want interchange и 2", v.MapID, len(shotMarkers(v)))
	}
	if c.Snapshot().Manual {
		t.Error("Manual после возврата к игре")
	}
}

func TestSelectSameMapKeepsRoute(t *testing.T) {
	c := inRaid()
	c.OnShot(shot1, at(21, 6, 30))
	c.SelectMap("maps/shopping_mall.bundle")
	if n := len(shotMarkers(c.View())); n != 1 {
		t.Errorf("выбрана карта рейда — маршрут виден, точек %d", n)
	}
}

func TestGameMapEndsManualChoice(t *testing.T) {
	c := New(fakeSrc{})
	c.SelectMap("maps/sandbox_start_preset.bundle")
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvMap, At: at(21, 4, 44), Bundle: "maps/shopping_mall.bundle"})
	if s := c.Snapshot(); s.Manual || s.MapID != "interchange" {
		t.Errorf("игра загрузила карту — показывать её: %+v", s)
	}
}

func TestSelectMapNewContentGen(t *testing.T) {
	c := New(fakeSrc{})
	g := c.View().ContentGen
	c.SelectMap("maps/shopping_mall.bundle")
	if c.View().ContentGen == g {
		t.Error("ContentGen не изменился — фронтенд не перечитает метки")
	}
}

func TestSetSourceKeepsManualChoice(t *testing.T) {
	c := New(fakeSrc{})
	c.SelectMap("maps/shopping_mall.bundle")
	c.SetSource(fakeSrc{}) // фоновое обновление данных
	if s := c.Snapshot(); !s.Manual || s.MapID != "interchange" {
		t.Errorf("Snapshot = %+v", s)
	}
}

func TestSelectUnsupportedMap(t *testing.T) {
	c := New(fakeSrc{})
	c.SelectMap("maps/laboratory_preset.bundle")
	if s := c.Snapshot(); s.Supported || !strings.Contains(s.Message, "Лаборатория") {
		t.Errorf("Snapshot = %+v", s)
	}
}
