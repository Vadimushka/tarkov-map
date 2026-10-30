package core

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Vadimushka/tarkov-map/internal/data"
	"github.com/Vadimushka/tarkov-map/internal/gamelog"
)

// Выгрузка вида и меток реальных карт для проверки фронтенда в браузере:
// TM_REAL_DATA=<кэш приложения> TM_DUMP=<папка>. Кэш копируется во временную папку
// и не меняется; недостающие items_ru и SVG докачиваются из сети.
func TestDumpRealContent(t *testing.T) {
	src, out := os.Getenv("TM_REAL_DATA"), os.Getenv("TM_DUMP")
	if src == "" || out == "" {
		t.Skip("только с TM_REAL_DATA и TM_DUMP")
	}
	dir := t.TempDir()
	for _, name := range []string{"maps.json", "pve_maps.json", "pve_maps_ru.json", "pve_items_ru.json", "svg/Interchange.svg"} {
		copyFile(t, filepath.Join(src, name), filepath.Join(dir, name))
	}
	store := data.NewStore(dir, &http.Client{Timeout: 60 * time.Second}, time.Now)
	if err := store.Load("pve", false); err != nil {
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

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	in, err := os.Open(from)
	if err != nil {
		return // необязательный файл (items_ru, SVG) докачается
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(to)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := io.Copy(f, in); err != nil {
		t.Fatal(err)
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
