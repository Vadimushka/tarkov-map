package main

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io/fs"
	"log"
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

	// задания по логам: режим → id → "start" | "fail" | "done"; всё до questsAt уже учёл
	// разбор логов на старте, а хвост текущей сессии с начала повторил бы это без режима
	quests      map[string]map[string]string
	questsAt    time.Time
	questsDirty bool

	tmu     sync.Mutex // tailer и watcher
	smu     sync.Mutex // запись settings.json
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
	var snap fs.FS // nil — снимок не открылся: в статусе будет «нет данных»
	if zr, err := zip.NewReader(bytes.NewReader(seedZip), int64(len(seedZip))); err == nil {
		snap = zr
	} else {
		log.Printf("снимок данных не открылся: %v", err) // целость проверяет TestSeedZipComplete
	}
	store := data.NewStore(snap)
	a := &App{
		flagGame: flagGame, flagShots: flagShots, cfgDir: cfgDir,
		store: store, core: core.New(store),
		set: settings.Load(cfgDir), mode: "pve", loading: true,
	}
	return a
}

// startup: данные из снимка, затем реплей текущей сессии логов и скринов и слежение.
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
	ctx, dirty, quests := a.ctx, a.dirty, a.questsDirty
	a.dirty, a.questsDirty = false, false
	a.mu.Unlock()
	if dirty && ctx != nil {
		runtime.EventsEmit(ctx, "snapshot", a.GetSnapshot())
		runtime.EventsEmit(ctx, "view")
	}
	if quests && ctx != nil {
		runtime.EventsEmit(ctx, "quests")
	}
}

func (a *App) markDirty() {
	a.mu.Lock()
	a.dirty = true
	a.mu.Unlock()
}

// useData делает текущими данные режима и обновляет сообщение об ошибке.
func (a *App) useData(mode string) {
	a.setDataErr(a.store.Load(mode))
	a.core.SetSource(a.store)
}

func (a *App) setDataErr(err error) {
	msg := ""
	if errors.Is(err, data.ErrNoData) {
		msg = "Нет данных карт: программа собрана без них, нужна новая сборка."
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
		a.questsDirty = true // у PvE и PvP свои задания
		a.mu.Unlock()
	case gamelog.EvMap:
		// данные режима — к загрузке карты: на входе в игру режим мигает Pve → Regular → Pve
		a.mu.Lock()
		mode := a.mode
		a.mu.Unlock()
		a.useData(mode)
	case gamelog.EvQuest:
		a.mu.Lock()
		if e.At.After(a.questsAt) {
			if a.quests == nil {
				a.quests = map[string]map[string]string{}
			}
			if a.quests[a.mode] == nil {
				a.quests[a.mode] = map[string]string{}
			}
			a.quests[a.mode][e.QuestID] = e.Status
			a.questsDirty = true
		}
		a.mu.Unlock()
		return
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
	quests := map[string]map[string]string{}
	if game != "" {
		quests = gamelog.ScanQuests(filepath.Join(game, "Logs"))
	}
	a.mu.Lock()
	a.quests, a.questsAt, a.questsDirty = quests, time.Now(), true
	a.mu.Unlock()
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

// GetQuestStatus — задания режима игры по логам: id → "start" | "fail" | "done".
func (a *App) GetQuestStatus() map[string]string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := map[string]string{}
	for id, st := range a.quests[a.mode] {
		out[id] = st
	}
	return out
}

func (a *App) SetHiddenTasks(ids []string) error {
	return a.updateSettings(func(s *settings.Settings) { s.HiddenTasks = ids })
}

// SetPresets сохраняет наборы слоёв: имя без пробелов по краям, без имени — не набор,
// у одинаковых имён остаётся последний (на месте первого).
func (a *App) SetPresets(presets []settings.Preset) error {
	var out []settings.Preset
	at := map[string]int{}
	for _, p := range presets {
		p.Name = strings.TrimSpace(p.Name)
		if p.Name == "" {
			continue
		}
		if i, ok := at[p.Name]; ok {
			out[i] = p
			continue
		}
		at[p.Name] = len(out)
		out = append(out, p)
	}
	return a.updateSettings(func(s *settings.Settings) { s.Presets = out })
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

// GetVersion — версия программы для «О программе».
func (a *App) GetVersion() string {
	return version
}

func (a *App) updateSettings(fn func(*settings.Settings)) error {
	a.smu.Lock() // правка и запись — одна за другой, иначе старая версия может записаться последней
	defer a.smu.Unlock()
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
