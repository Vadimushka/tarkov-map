package data

import (
	"os"
	"path/filepath"
	"testing"
)

// Разбор полных реальных файлов: TM_REAL_DATA=<папка с maps.json, pve_maps.json, pve_maps_ru.json>
func TestParseRealFiles(t *testing.T) {
	dir := os.Getenv("TM_REAL_DATA")
	if dir == "" {
		t.Skip("только с TM_REAL_DATA")
	}
	defs, err := parseDefs(filepath.Join(dir, "maps.json"))
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
	t.Logf("SVG-карт: %d, карт в данных: %d", len(defs), len(maps))
}
