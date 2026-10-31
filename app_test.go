package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Vadimushka/tarkov-map/internal/core"
	"github.com/Vadimushka/tarkov-map/internal/data"
	"github.com/Vadimushka/tarkov-map/internal/gamelog"
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
		if s.TaskCount() == 0 {
			t.Errorf("%s: нет заданий", mode)
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

// Версию подставляет сборка (-ldflags "-X main.version=…"); «О программе» показывает её.
func TestGetVersion(t *testing.T) {
	old := version
	version = "v1.2.3"
	t.Cleanup(func() { version = old })
	if got := (&App{}).GetVersion(); got != "v1.2.3" {
		t.Errorf("GetVersion() = %q", got)
	}
}

// Задания: прежние — из разбора логов на старте, новые — из хвоста, в режиме, который сейчас в
// игре. Чтение текущей сессии с начала повторяет старые уведомления — их режим уже неизвестен,
// и они не должны портить разобранное.
func TestQuestStatus(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 10, 31, h, m, 0, 0, time.Local) }
	a := &App{core: core.New(nil), mode: "pve", questsAt: at(12, 0),
		quests: map[string]map[string]string{"pve": {"q1": "start"}, "regular": {"q3": "start"}}}
	a.onLogEvent(gamelog.Event{Kind: gamelog.EvQuest, At: at(11, 0), QuestID: "q3", Status: "start"})
	a.onLogEvent(gamelog.Event{Kind: gamelog.EvQuest, At: at(12, 5), QuestID: "q1", Status: "done"})
	a.onLogEvent(gamelog.Event{Kind: gamelog.EvQuest, At: at(12, 6), QuestID: "q2", Status: "start"})
	if got := fmt.Sprint(a.GetQuestStatus()); got != "map[q1:done q2:start]" {
		t.Errorf("PvE = %s", got)
	}
	a.onLogEvent(gamelog.Event{Kind: gamelog.EvMode, Mode: "regular"})
	if got := fmt.Sprint(a.GetQuestStatus()); got != "map[q3:start]" {
		t.Errorf("PvP = %s", got)
	}
	if !a.questsDirty {
		t.Error("фронтенд не узнает о новых заданиях")
	}
}

func TestSetHiddenTasks(t *testing.T) {
	dir := t.TempDir()
	a := &App{cfgDir: dir, set: settings.Load(dir)}
	if err := a.SetHiddenTasks([]string{"t1", "t2"}); err != nil {
		t.Fatal(err)
	}
	if got := settings.Load(dir).HiddenTasks; strings.Join(got, ",") != "t1,t2" {
		t.Errorf("на диске %v", got)
	}
}
