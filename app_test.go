package main

import (
	"fmt"
	"sync"
	"testing"

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
