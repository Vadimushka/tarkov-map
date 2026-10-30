package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"sync"
	"testing"
	"time"

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

// Снимок внутри exe цел и полон: битый или неполный zip означал бы exe, который без
// сети молча не работает, — ради этого снимок и встраивается.
func TestSeedZipComplete(t *testing.T) {
	zr, err := zip.NewReader(bytes.NewReader(seedZip), int64(len(seedZip)))
	if err != nil {
		t.Fatalf("снимок не открывается: %v", err)
	}
	s := data.NewStore(t.TempDir(), nil, time.Now)
	s.Seed = zr
	for _, mode := range []string{"pve", "regular"} {
		if err := s.LoadCached(mode); err != nil {
			t.Errorf("%s: %v", mode, err)
		}
		if _, err := fs.Stat(zr, mode+"_items_ru.json"); err != nil {
			t.Errorf("%s: нет названий предметов", mode)
		}
	}
	defs := s.SVGDefs()
	if len(defs) < 10 {
		t.Errorf("SVG-карт в maps.json: %d", len(defs))
	}
	for _, d := range defs {
		if _, err := fs.Stat(zr, "svg/"+path.Base(d.SVGPath)); err != nil {
			t.Errorf("в снимке нет %s", path.Base(d.SVGPath))
		}
	}
}
