package core

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Vadimushka/tarkov-map/internal/gamelog"
	"github.com/Vadimushka/tarkov-map/internal/mapgeo"
)

func TestContentProjectsAndResolves(t *testing.T) {
	c := New(fakeSrc{})
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvMap, Bundle: "maps/shopping_mall.bundle"})
	m := c.Content()
	px, py := mapgeo.Project(ic, 10, 20)
	if len(m.Containers) != 1 || m.Containers[0].PX != px || m.Containers[0].PY != py ||
		m.Containers[0].Group != "weapon" || m.Containers[0].Type != "weapon-box" || m.Containers[0].Layer != "Ground_Level" {
		t.Errorf("Containers = %+v", m.Containers)
	}
	if len(m.Loot) != 1 || m.Loot[0].Layer != "First_Floor" {
		t.Errorf("Loot = %+v", m.Loot)
	}
	if len(m.Items) != 1 || m.Items["i_led"].Short != "LEDX" || m.ItemsMissing {
		t.Errorf("Items = %+v, missing=%v", m.Items, m.ItemsMissing)
	}
	if len(m.Locks) != 1 || m.Locks[0].Key != "Ключ от КПП" {
		t.Errorf("Locks = %+v", m.Locks)
	}
	if len(m.Hazards) != 1 || len(m.Hazards[0].Outline) != 3 {
		t.Errorf("Hazards = %+v", m.Hazards)
	}
	if len(m.Bosses) != 1 || m.Bosses[0].Name != "Килла" || m.Bosses[0].SpotChance != 0.33 {
		t.Errorf("Bosses = %+v", m.Bosses)
	}
	layer := map[string]string{}
	for _, p := range m.Places {
		layer[p.Text] = p.Layer
	}
	if layer["IDEA"] != "First_Floor" || layer["Электростанция"] != "Ground_Level" {
		t.Errorf("этажи подписей: %v", layer)
	}
}

func TestContentItemsMissing(t *testing.T) {
	c := New(noItemsSrc{})
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvMap, Bundle: "maps/shopping_mall.bundle"})
	if m := c.Content(); !m.ItemsMissing || m.Items == nil {
		t.Errorf("ItemsMissing=%v Items=%v", m.ItemsMissing, m.Items)
	}
}

func TestContentEmptyForMapWithoutContent(t *testing.T) {
	c := New(fakeSrc{})
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvMap, Bundle: "maps/sandbox_start_preset.bundle"})
	b, _ := json.Marshal(c.Content())
	for _, k := range []string{"Places", "Containers", "Loot", "Locks", "Switches", "Hazards", "Weapons", "Bosses"} {
		if !strings.Contains(string(b), `"`+k+`":[]`) {
			t.Errorf("%s должен быть пустым списком: %s", k, b)
		}
	}
	if !strings.Contains(string(b), `"Items":{}`) || strings.Contains(string(b), `"ItemsMissing":true`) {
		t.Errorf("Items/ItemsMissing: %s", b)
	}
}

func TestContentGenGrowsOnSetSource(t *testing.T) {
	c := New(fakeSrc{})
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvMap, Bundle: "maps/shopping_mall.bundle"})
	g := c.View().ContentGen
	c.SetSource(fakeSrc{}) // фоновое обновление данных
	if v := c.View(); v.ContentGen <= g || c.Content().Gen != v.ContentGen {
		t.Errorf("ContentGen %d → %d, Content().Gen %d", g, v.ContentGen, c.Content().Gen)
	}
}
