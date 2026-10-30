package data

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Vadimushka/tarkov-map/internal/mapgeo"
)

// Разбор полных реальных файлов: TM_REAL_DATA=<папка с maps.json, pve_maps.json, pve_maps_ru.json>
func TestParseRealFiles(t *testing.T) {
	dir := os.Getenv("TM_REAL_DATA")
	if dir == "" {
		t.Skip("только с TM_REAL_DATA")
	}
	defs, labels, err := parseDefs(filepath.Join(dir, "maps.json"))
	if err != nil {
		t.Fatal(err)
	}
	maps, err := parseMaps(filepath.Join(dir, "pve_maps.json"), filepath.Join(dir, "pve_maps_ru.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"customs", "factory", "ground-zero", "interchange", "lighthouse", "reserve", "shoreline", "streets-of-tarkov", "terminal", "woods"} {
		if _, ok := defs[name]; !ok {
			t.Errorf("нет SVG-карты %s", name)
		}
	}
	ic, ok := maps["maps/shopping_mall.bundle"]
	if !ok || len(ic.Extracts) != 9 || len(ic.Transits) != 2 {
		t.Errorf("Развязка: ok=%v выходов=%d переходов=%d", ok, len(ic.Extracts), len(ic.Transits))
	}
	// числа — по кэшу PvE от 29.10.2026; данные tarkov.dev меняются — сверять с кэшем
	if len(ic.Containers) != 823 || len(ic.Loose) != 492 || len(ic.Locks) != 15 || len(ic.Hazards) != 25 ||
		len(ic.Switches) != 6 || len(ic.Weapons) != 2 || len(ic.Bosses) != 2 {
		t.Errorf("метки Развязки: контейнеров %d, лута %d, замков %d, зон %d, рубильников %d, пулемётов %d, боссов %d",
			len(ic.Containers), len(ic.Loose), len(ic.Locks), len(ic.Hazards), len(ic.Switches), len(ic.Weapons), len(ic.Bosses))
	}
	floors := map[string]int{}
	for _, l := range labels["interchange"] {
		if l.Bottom != nil && l.Top != nil {
			floors[mapgeo.LayerOfSpan(defs["interchange"], l.X, l.Z, *l.Bottom, *l.Top)]++
		} else {
			floors[defs["interchange"].SVGLayer]++
		}
	}
	if floors["Ground_Level"] != 14 || floors["First_Floor"] != 33 || floors["Second_Floor"] != 30 {
		t.Errorf("этажи подписей Развязки: %v", floors)
	}
	for _, l := range labels["shoreline"] {
		if l.Bottom != nil && l.Top != nil && mapgeo.LayerOfSpan(defs["shoreline"], l.X, l.Z, *l.Bottom, *l.Top) != defs["shoreline"].SVGLayer {
			t.Errorf("Берег: подпись %q не на земле", l.Text)
		}
	}
	t.Logf("SVG-карт: %d, карт в данных: %d", len(defs), len(maps))
}

// Список ручного выбора на реальных данных: TM_REAL_DATA — папка кэша приложения.
func TestRealMapsList(t *testing.T) {
	dir := os.Getenv("TM_REAL_DATA")
	if dir == "" {
		t.Skip("только с TM_REAL_DATA")
	}
	s := NewStore(dir, nil, nil)
	if err := s.parse("pve"); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, m := range s.Maps() {
		t.Logf("%-40s %-22s %s", m.Scene, m.ID, m.Name)
		if names[m.Name] {
			t.Errorf("два пункта «%s» — в списке не различить", m.Name)
		}
		names[m.Name] = true
	}
	if !names["Улицы Таркова"] || !names["Развязка"] {
		t.Errorf("нет Улиц или Развязки: %v", names)
	}
}
