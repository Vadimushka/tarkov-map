package core

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Vadimushka/tarkov-map/internal/data"
	"github.com/Vadimushka/tarkov-map/internal/gamelog"
)

// Выгрузка вида и меток реальных карт для проверки фронтенда в браузере: TM_DUMP=<папка>.
// Данные — снимок assets/data.zip, тот же, что внутри exe.
func TestDumpRealContent(t *testing.T) {
	out := os.Getenv("TM_DUMP")
	if out == "" {
		t.Skip("только с TM_DUMP")
	}
	zr, err := zip.OpenReader("../../assets/data.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	store := data.NewStore(zr)
	if err := store.Load("pve"); err != nil {
		t.Fatal(err)
	}
	for scene, name := range map[string]string{
		"maps/shopping_mall.bundle":    "interchange",
		"maps/city_preset.bundle":      "streets-of-tarkov",
		"maps/shoreline_preset.bundle": "shoreline",
	} {
		c := New(store)
		c.OnLogEvent(gamelog.Event{Kind: gamelog.EvMap, Bundle: scene})
		def, ok := store.MapDef(name)
		if !ok {
			t.Fatalf("нет SVG-карты %s", name)
		}
		svg, err := store.SVG(def)
		if err != nil {
			t.Fatal(err)
		}
		d := filepath.Join(out, name)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		writeJSON(t, filepath.Join(d, "view.json"), c.View())
		m := c.Content()
		writeJSON(t, filepath.Join(d, "content.json"), m)
		if err := os.WriteFile(filepath.Join(d, "map.svg"), []byte(svg), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: подписей %d, контейнеров %d, лута %d, предметов %d (нет названий: %v), замков %d, зон %d, пулемётов %d, точек боссов %d",
			name, len(m.Places), len(m.Containers), len(m.Loot), len(m.Items), m.ItemsMissing, len(m.Locks), len(m.Hazards), len(m.Weapons), len(m.Bosses))
	}
}

func writeJSON(t *testing.T, file string, v any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, b, 0o644); err != nil {
		t.Fatal(err)
	}
}
