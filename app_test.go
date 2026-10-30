package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/Vadimushka/tarkov-map/internal/data"
	"github.com/Vadimushka/tarkov-map/internal/settings"
)

// Правки настроек из разных вызовов фронтенда идут параллельно: на диске должна
// оказаться последняя версия со всеми правками, а не та, что записалась последней.
func TestUpdateSettingsKeepsEveryChange(t *testing.T) {
	dir := t.TempDir()
	a := &App{cfgDir: dir, set: settings.Load(dir)}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			_ = a.updateSettings(func(s *settings.Settings) {
				f := map[string]bool{key: true}
				for k, v := range s.Filters {
					f[k] = v
				}
				s.Filters = f
			})
		}(fmt.Sprint("k", i))
	}
	wg.Wait()
	if n := len(settings.Load(dir).Filters); n != 50 {
		t.Errorf("на диске %d флажков из 50", n)
	}
}

// Снимок внутри exe цел и полон: других данных у программы нет, и битый или неполный
// zip означал бы exe без карт.
func TestSeedZipComplete(t *testing.T) {
	zr, err := zip.NewReader(bytes.NewReader(seedZip), int64(len(seedZip)))
	if err != nil {
		t.Fatalf("снимок не открывается: %v", err)
	}
	s := data.NewStore(zr)
	for _, mode := range []string{"pve", "regular"} {
		if err := s.Load(mode); err != nil {
			t.Errorf("%s: %v", mode, err)
		}
		if s.ItemCount() == 0 {
			t.Errorf("%s: нет названий предметов", mode)
		}
	}
	defs := s.SVGDefs()
	if len(defs) < 10 {
		t.Errorf("SVG-карт в maps.json: %d", len(defs))
	}
	for _, d := range defs {
		if svg, err := s.SVG(d); err != nil || !strings.Contains(svg, "<svg") {
			t.Errorf("в снимке нет %s", data.SVGName(d))
		}
	}
}
