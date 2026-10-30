package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Vadimushka/tarkov-map/internal/core"
	"github.com/Vadimushka/tarkov-map/internal/data"
	"github.com/Vadimushka/tarkov-map/internal/gamelog"
	"github.com/Vadimushka/tarkov-map/internal/paths"
	"github.com/Vadimushka/tarkov-map/internal/settings"
	"github.com/Vadimushka/tarkov-map/internal/shots"
	"github.com/Vadimushka/tarkov-map/internal/winpos"
)

// Paths — найденные пути для статусной строки и настроек.
type Paths struct {
	Game, Shots           string
	GameFound, ShotsFound bool
	DetectedGame          string
	DetectedShots         string
}

// App — методы, доступные фронтенду через window.go.main.App.
type App struct {
	ctx                 context.Context
	flagGame, flagShots string
	cfgDir              string
	store               *data.Store
	sync                *data.Syncer
	core                *core.Core

	mu                sync.Mutex
	set               settings.Settings
	mode              string
	gameDir, shotsDir string
	detGame, detShots string
	detected          bool
	dataErr, watchErr string
	loading           bool
	dirty             bool

	tmu     sync.Mutex // tailer и watcher
	tailer  *gamelog.Tailer
	watcher *shots.Watcher

	shown sync.Once
}

// NewApp готовит всё, что нужно методам фронтенда: он может вызвать их
// раньше, чем закончится startup.
func NewApp(flagGame, flagShots string) *App {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	cfgDir := filepath.Join(base, "TarkovMap")
	store := data.NewStore(cfgDir, &http.Client{Timeout: 30 * time.Second}, time.Now)
	a := &App{
		flagGame: flagGame, flagShots: flagShots, cfgDir: cfgDir,
		store: store, core: core.New(store),
		set: settings.Load(cfgDir), mode: "pve", loading: true,
	}
	// фоновое обновление данных: метки пересчитываются по свежим данным
	a.sync = data.NewSyncer(store, func() {
		a.core.SetSource(a.store)
		a.markDirty()
	})
	return a
}

// startup: данные из кэша (сеть — в фоне или, без кэша, сразу), затем реплей
// текущей сессии логов и скринов и слежение.
func (a *App) startup(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	mode := a.mode
	a.mu.Unlock()
	a.useData(mode)
	a.startWatching()
	go a.loop()
}

// domReady показывает окно (оно стартует скрытым) сразу на сохранённом месте —
// без прыжка из центра экрана.
func (a *App) domReady(ctx context.Context) {
	a.shown.Do(func() {
		a.mu.Lock()
		w := a.set.Window
		a.mu.Unlock()
		if w != nil {
			winpos.Apply(*w)
		}
		runtime.WindowShow(ctx)
	})
}

// beforeClose запоминает положение и размер окна; false — закрытию не мешаем.
func (a *App) beforeClose(ctx context.Context) bool {
	if w, ok := winpos.Get(); ok {
		_ = a.updateSettings(func(s *settings.Settings) { s.Window = &w })
	}
	return false
}

func (a *App) loop() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-t.C:
			a.tmu.Lock()
			if a.tailer != nil {
				a.tailer.Poll()
			}
			a.tmu.Unlock()
			a.flush()
		}
	}
}

// flush сообщает фронтенду об изменениях не чаще раза в тик.
func (a *App) flush() {
	a.mu.Lock()
	ctx, dirty := a.ctx, a.dirty
	a.dirty = false
	a.mu.Unlock()
	if dirty && ctx != nil {
		runtime.EventsEmit(ctx, "snapshot", a.GetSnapshot())
		runtime.EventsEmit(ctx, "view")
	}
}

func (a *App) markDirty() {
	a.mu.Lock()
	a.dirty = true
	a.mu.Unlock()
}

// useData делает текущими данные режима (см. data.Syncer) и обновляет сообщение об ошибке.
func (a *App) useData(mode string) {
	a.setDataErr(a.sync.Use(mode))
	a.core.SetSource(a.store)
}

func (a *App) setDataErr(err error) {
	msg := ""
	if errors.Is(err, data.ErrNoData) {
		msg = "Нет данных карт: нет сети и нет кэша. Подключитесь к интернету и нажмите «Обновить данные»."
	} else if err != nil {
		msg = "Ошибка данных карт: " + err.Error()
	}
	a.mu.Lock()
	a.dataErr = msg
	a.loading = false
	a.dirty = true
	a.mu.Unlock()
}

func (a *App) onLogEvent(e gamelog.Event) {
	switch e.Kind {
	case gamelog.EvMode:
		a.mu.Lock()
		a.mode = e.Mode
		a.mu.Unlock()
	case gamelog.EvMap:
		// данные режима — к загрузке карты: на входе в игру режим мигает Pve → Regular → Pve
		a.mu.Lock()
		mode := a.mode
		a.mu.Unlock()
		a.useData(mode)
	}
	a.core.OnLogEvent(e)
	a.markDirty()
}

func (a *App) onShot(name string, at time.Time) {
	if a.core.OnShot(name, at) {
		a.markDirty()
	}
}

func (a *App) resolvePaths() (game, shotsDir string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.detected {
		a.detGame, a.detShots = paths.Detect()
		a.detected = true
	}
	game = firstNonEmpty(a.flagGame, a.set.GameDir, a.detGame)
	shotsDir = firstNonEmpty(a.flagShots, a.set.ShotsDir, a.detShots)
	a.gameDir, a.shotsDir = game, shotsDir
	return game, shotsDir
}

// startWatching (пере)запускает чтение логов и слежение за скриншотами
// и восстанавливает состояние текущего рейда.
func (a *App) startWatching() {
	game, shotsDir := a.resolvePaths()
	a.tmu.Lock()
	defer a.tmu.Unlock()
	if a.watcher != nil {
		a.watcher.Close()
		a.watcher = nil
	}
	a.tailer = nil
	if game != "" {
		a.tailer = gamelog.NewTailer(filepath.Join(game, "Logs"), a.onLogEvent)
		a.tailer.Poll() // текущая сессия — с начала
	}
	watchErr := ""
	if shotsDir != "" {
		a.replayShots(shotsDir)
		w, err := shots.NewWatcher(shotsDir, a.onShot)
		if err != nil {
			watchErr = "Не удалось следить за папкой скриншотов: " + err.Error()
		}
		a.watcher = w
	}
	a.mu.Lock()
	a.watchErr = watchErr
	a.dirty = true
	a.mu.Unlock()
}

// replayShots подаёт уже лежащие скрины по времени изменения:
// в маршрут попадут только сделанные после начала текущего рейда.
func (a *App) replayShots(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type f struct {
		name string
		mod  time.Time
	}
	var files []f
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".png") {
			continue
		}
		if info, err := e.Info(); err == nil {
			files = append(files, f{e.Name(), info.ModTime()})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	for _, x := range files {
		a.core.OnShot(x.name, x.mod)
	}
}

// --- методы для фронтенда ---

func (a *App) GetSnapshot() core.Snapshot {
	s := a.core.Snapshot()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.loading {
		s.Message = "Загружаем данные карт…"
	} else if a.dataErr != "" {
		s.Message = a.dataErr
	} else if a.watchErr != "" && s.Message == "" {
		s.Message = a.watchErr
	}
	return s
}

func (a *App) GetView() core.MapView {
	return a.core.View()
}

// GetMapContent — метки вкладки «Лут и места» текущей карты; фронтенд берёт их
// заново, когда меняется MapView.ContentGen.
func (a *App) GetMapContent() core.MapContent {
	return a.core.Content()
}

// GetMaps — карты для ручного выбора.
func (a *App) GetMaps() []data.MapRef {
	return a.store.Maps()
}

// SelectMap показывает карту по сцене; "" — карта, которую загрузила игра.
// Выбор действует до загрузки игрой следующей карты.
func (a *App) SelectMap(scene string) {
	a.core.SelectMap(scene)
	a.markDirty()
}

func (a *App) GetSVG(mapID string) (string, error) {
	def, ok := a.store.MapDef(mapID)
	if !ok {
		return "", errors.New("нет SVG для карты " + mapID)
	}
	return a.store.SVG(def)
}

func (a *App) GetSettings() settings.Settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.set
}

func (a *App) SetSide(side string) error {
	if side != "pmc" && side != "scav" {
		return errors.New("сторона должна быть pmc или scav")
	}
	return a.updateSettings(func(s *settings.Settings) { s.Side = side })
}

func (a *App) SetFilters(f map[string]bool) error {
	return a.updateSettings(func(s *settings.Settings) { s.Filters = f })
}

func (a *App) SetLootQuery(q string) error {
	return a.updateSettings(func(s *settings.Settings) { s.LootQuery = q })
}

// SetPaths сохраняет ручные пути ("" — автопоиск) и перезапускает слежение.
func (a *App) SetPaths(gameDir, shotsDir string) error {
	if err := a.updateSettings(func(s *settings.Settings) {
		s.GameDir, s.ShotsDir = strings.TrimSpace(gameDir), strings.TrimSpace(shotsDir)
	}); err != nil {
		return err
	}
	a.startWatching()
	a.flush()
	return nil
}

// RefreshData — кнопка «Обновить данные»: данные режима и SVG текущей карты из сети
// сейчас; без сети всё остаётся на кэше, а ошибка уходит в ответ.
func (a *App) RefreshData() error {
	a.mu.Lock()
	mode := a.mode
	a.mu.Unlock()
	err := a.sync.Refresh(mode)
	switch {
	case err == nil:
		a.setDataErr(nil)
	case a.sync.Mode() != mode: // данных режима нет совсем — это в статус
		a.setDataErr(data.ErrNoData)
	}
	a.core.SetSource(a.store)
	if id := a.core.Snapshot().MapID; err == nil && id != "" {
		if def, ok := a.store.MapDef(id); ok {
			err = a.store.RefreshSVG(def)
		}
	}
	a.markDirty()
	a.flush()
	return err
}

func (a *App) GetPaths() Paths {
	a.mu.Lock()
	defer a.mu.Unlock()
	return Paths{
		Game: a.gameDir, Shots: a.shotsDir,
		GameFound: dirExists(filepath.Join(a.gameDir, "Logs")), ShotsFound: dirExists(a.shotsDir),
		DetectedGame: a.detGame, DetectedShots: a.detShots,
	}
}

func (a *App) GetDataUpdated() time.Time {
	return a.store.Updated()
}

func (a *App) updateSettings(fn func(*settings.Settings)) error {
	a.mu.Lock()
	fn(&a.set)
	s := a.set
	a.mu.Unlock()
	return settings.Save(a.cfgDir, s)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func dirExists(p string) bool {
	if p == "" {
		return false
	}
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
