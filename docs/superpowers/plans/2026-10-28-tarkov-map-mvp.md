# Tarkov Map MVP — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `TarkovMap.exe`, который открывает окно с картой текущей локации, сам переключает карту по логам игры и по каждому скриншоту ставит метку с направлением взгляда, маршрут рейда и выходы с фильтром.

**Architecture:** Go-бэкенд (Wails v2) читает логи игры и папку скриншотов, держит кэш данных tarkov.dev и сам пересчитывает игровые координаты в координаты карты; фронтенд на чистых HTML/JS/SVG (развитие спайка) только рисует готовую «модель вида». Логика рейда собрана в чистом пакете `internal/core`, который тестируется без Wails.

**Tech Stack:** Go (последняя стабильная, в WSL Ubuntu), Wails v2 (`github.com/wailsapp/wails/v2@latest`), `github.com/fsnotify/fsnotify`, `golang.org/x/sys/windows`; фронтенд без сборщика и без npm.

**Spec:** `docs/superpowers/specs/2026-10-28-tarkov-map-design.md` — читать вместе с планом.

**Вне этого плана (следующие планы):** учёт квестов и квестовый слой; лут, замки, опасные зоны, рубильники и поиск по предметам; тайлы этажей; GitHub Actions.

## Global Constraints

- Приложение **только читает** файлы логов и **имена** файлов скриншотов. Никакого доступа к процессу/памяти игры, никаких окон поверх игры.
- Скриншоты **никогда** не удаляются и не изменяются; сам файл не открывается (его может удалить скрипт TM сразу после создания).
- Сеть — только HTTP GET к `json.tarkov.dev`, `assets.tarkov.dev`, `raw.githubusercontent.com`. Ничего не отправляется.
- Цель сборки — `windows/amd64`, `CGO_ENABLED=0`, один exe. Сборка и юнит-тесты — в WSL; Windows-специфичные тесты — через `scripts/wintest.sh` (тестовый exe запускается из WSL через interop).
- Кэш и состояние — в `%LOCALAPPDATA%\TarkovMap\` (`os.UserCacheDir()` + `TarkovMap`).
- Данные обновляются при запуске, если кэшу больше 24 ч; без сети — работа на кэше.
- SVG карт и данные tarkov.dev **не коммитятся**.
- Весь текст интерфейса — на русском.
- Переводы строк — LF (`.gitattributes` уже есть). Коммиты — на русском, с трейлером `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Режимы данных называются как сегменты URL tarkov.dev: `"pve"` и `"regular"`.

## Review Focus

1. **Скрипт TM удаляет скрин сразу после создания** → позиция всё равно должна попасть на карту (по имени из события создания). Тест: Task 3, `TestWatcherCreateThenDelete`.
2. **Приложение запущено посреди рейда** → карта, режим и маршрут восстанавливаются из текущей сессии логов и скринов после последнего `GameStarted`. Тесты: Task 4 `TestTailerReadsSessionFromStart`, Task 7 `TestRestoreMidRaid`.
3. **Первый запуск без интернета** → понятное сообщение, без падения; с кэшем — работа на кэше. Тесты: Task 6 `TestLoadOfflineWithCache`, `TestLoadOfflineNoCache`.
4. **Старые скрины в папке и скрины до начала рейда** → не попадают в маршрут. Тест: Task 7 `TestShotBeforeRaidIgnored`.
5. **Карта без SVG (Лаборатория, Ледокол, Лабиринт)** → сообщение «Карта «…» пока не поддерживается», без падения. Тест: Task 7 `TestUnsupportedMap`.

---

## Структура файлов

```
main.go                      Wails: окно, embed фронтенда, флаги -game/-shots
app.go                       Wails-обвязка: старт, циклы, методы для фронтенда, события
internal/mapgeo/geo.go       типы карты, Project, ImageRect, LayerOf, Forward
internal/shots/parse.go      ParseName, Shot.Key
internal/shots/watcher.go    слежение за *.png
internal/gamelog/parse.go    ParseLine → Event
internal/gamelog/tailer.go   выбор сессии, чтение с начала + хвост
internal/paths/paths.go      ParseLibraryFolders, ChooseGameDir (переносимо)
internal/paths/detect_windows.go / detect_other.go   Detect()
internal/data/store.go       загрузка/кэш/разбор данных, SVG
internal/data/testdata/      урезанные реальные JSON и мини-SVG
internal/settings/settings.go  settings.json
internal/core/core.go        логика рейда, Snapshot, View
frontend/dist/index.html, style.css, app.js, map.js
scripts/build.sh, scripts/wintest.sh, scripts/smoke.ps1
```

---

### Task 1: Каркас и проверка кросс-сборки

**Files:**
- Create: `go.mod` (module `github.com/Vadimushka/tarkov-map`), `main.go`, `app.go`, `frontend/dist/index.html`, `scripts/build.sh`, `scripts/wintest.sh`, `.gitignore` (`build/`, `*.exe`)

**Interfaces:**
- Produces: `type App struct{ ctx context.Context }`, `func NewApp() *App`, `func (a *App) startup(ctx context.Context)`, `func (a *App) Ping() string` (возвращает `"pong"`). Скрипты `scripts/build.sh` → `build/bin/TarkovMap.exe`; `scripts/wintest.sh <pkgdir>`.

- [ ] **Step 1: Поставить Go в WSL без sudo**

Скачать последний `go<ver>.linux-amd64.tar.gz` с go.dev, распаковать в `~/.local/go`, добавить в `~/.profile`: `export PATH=$HOME/.local/go/bin:$HOME/go/bin:$PATH`.
Run: `wsl -e bash -lc 'go version'` — Expected: `go version go1.x linux/amd64`.

- [ ] **Step 2: Каркас Wails без CLI**

`main.go`: `//go:embed all:frontend/dist`, `wails.Run(&options.App{Title: "Tarkov Map", Width: 1280, Height: 800, BackgroundColour: &options.RGBA{R: 20, G: 19, B: 15, A: 1}, AssetServer: &assetserver.Options{Assets: assets}, OnStartup: app.startup, Bind: []interface{}{app}})`. `index.html` по загрузке вызывает `window.go.main.App.Ping()` и выводит результат крупно на странице. Сгенерированные биндинги (`wailsjs/`) не используются — фронтенд обращается к `window.go.main.App.*` и `window.runtime.*` напрямую.

`scripts/build.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags desktop,production \
  -ldflags "-s -w -H windowsgui" -o build/bin/TarkovMap.exe .
```
`scripts/wintest.sh` — `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o build/test/<pkg>.test.exe ./<pkgdir>` и запуск `./build/test/<pkg>.test.exe -test.v`, код возврата — как у теста.

- [ ] **Step 3: Собрать в WSL**

Run: `wsl -e bash -lc 'cd /mnt/c/Users/vadimushka_d/Projects/tarkov-map && go mod tidy && ./scripts/build.sh && ls -la build/bin'`
Expected: `TarkovMap.exe` размером в несколько МБ, без ошибок.

- [ ] **Step 4: Запустить в Windows**

Двойной клик по `build\bin\TarkovMap.exe` (или `Start-Process` из PowerShell). Expected: окно «Tarkov Map», на странице `pong`. Сделать скриншот окна для отчёта.
**Если не собирается или окно не открывается** — остановиться и сообщить: по спеке запасной путь — сборка в GitHub Actions на `windows-latest`, это решает владелец.

- [ ] **Step 5: Commit** — «Каркас приложения на Wails и скрипты сборки из WSL».

---

### Task 2: Пересчёт координат (`internal/mapgeo`)

**Files:**
- Create: `internal/mapgeo/geo.go`, `internal/mapgeo/geo_test.go`

**Interfaces:**
- Produces:
```go
type Vec3 struct{ X, Y, Z float64 }
type Extent struct {
    Height [2]float64      // [lo, hi)
    Bounds [][2][2]float64 // прямоугольники, точки в порядке [x, z]
}
type Layer struct{ Name, SVGLayer string; Extents []Extent }
type MapDef struct {
    Key                string
    Transform          []float64     // [sx, mx, sy, my]; пусто = [1,0,1,0]
    CoordinateRotation float64       // градусы
    Bounds             [2][2]float64 // [x, z]
    SVGBounds          *[2][2]float64
    SVGPath, SVGLayer  string
    Layers             []Layer
}
type Rect struct{ X, Y, W, H float64 }
func Project(md MapDef, x, z float64) (px, py float64)
func ImageRect(md MapDef) Rect
func LayerOf(md MapDef, p Vec3) string
func Forward(qx, qy, qz, qw float64) (fx, fz float64)
```

- [ ] **Step 1: Тесты** (фикстура `interchange` — значения из maps.json, см. спеку)

```go
var ic = MapDef{Key: "interchange", Transform: []float64{0.265, 150.6, 0.265, 134.6},
    CoordinateRotation: 180, Bounds: [2][2]float64{{598, -442}, {-433, 426}}, SVGLayer: "Ground_Level",
    Layers: []Layer{
        {Name: "2nd Floor", SVGLayer: "First_Floor", Extents: []Extent{{Height: [2]float64{25, 34}, Bounds: [][2][2]float64{{{120, 218}, {-222, -327}}}}}},
        {Name: "3rd Floor", SVGLayer: "Second_Floor", Extents: []Extent{{Height: [2]float64{34, 1000}, Bounds: [][2][2]float64{{{120, 218}, {-222, -327}}}}}},
    }}

func TestProjectInterchange(t *testing.T) // Project(ic, -51.12, 47.90) == (164.1468, 147.2935) ±1e-6
func TestProjectNoRotation(t *testing.T)  // Project(MapDef{Transform: {1,0,1,0}}, 10, 20) == (10, -20)
func TestImageRect(t *testing.T)          // ImageRect(ic) == Rect{-7.87, 17.47, 273.215, 230.02} ±1e-6
func TestLayerOf(t *testing.T)
// (0,30,0)→"First_Floor"; (0,25,0)→"First_Floor"; (0,34,0)→"Second_Floor"; (0,40,0)→"Second_Floor";
// (0,22,0)→"Ground_Level"; (-312.74,25.2,-111.95)→"Ground_Level" (вне прямоугольника ТЦ)
func TestForward(t *testing.T)            // Forward(0,0.70711,0,0.70711) ≈ (1,0) ±1e-4; Forward(0,0,0,1) == (0,1)
```

- [ ] **Step 2:** `go test ./internal/mapgeo/` — FAIL (нет функций).
- [ ] **Step 3: Реализовать** по формулам спеки (раздел «Игровые координаты → точка на карте»). `ImageRect` — min/max по осям из `project(minX, maxZ)` и `project(maxX, minZ)` от `SVGBounds`, если задан, иначе `Bounds`. `Forward`: `fx = 2(qx·qz + qw·qy)`, `fz = 1 − 2(qx² + qy²)`.
- [ ] **Step 4:** `go test ./internal/mapgeo/` — PASS.
- [ ] **Step 5: Commit** — «Пересчёт игровых координат в координаты карты».

---

### Task 3: Скриншоты (`internal/shots`)

**Files:**
- Create: `internal/shots/parse.go`, `internal/shots/watcher.go`, `internal/shots/shots_test.go`

**Interfaces:**
- Consumes: `mapgeo.Vec3`
- Produces:
```go
type Shot struct {
    Name string       // имя файла
    Date, Time string // "2026-10-28", "21:06"
    Pos  mapgeo.Vec3
    Q    [4]float64   // qx, qy, qz, qw
}
func ParseName(name string) (Shot, bool)
func (s Shot) Key() string // одинаков для "(0)" и "(1)" с теми же координатами и поворотом
type Watcher struct{ /* fsnotify */ }
func NewWatcher(dir string, onShot func(name string, at time.Time)) (*Watcher, error)
func (w *Watcher) Close() error
```

- [ ] **Step 1: Тесты разбора**

```go
const real0 = "2026-10-28[21-06]_-312.74, 23.88, -111.95_-0.04637, 0.85289, 0.07698, 0.51430_9.75 (0).png"
func TestParseName(t *testing.T)   // ok; Date "2026-10-28"; Time "21:06"; Pos {-312.74, 23.88, -111.95}; Q {-0.04637, 0.85289, 0.07698, 0.51430}
func TestParseNameDuplicateKey(t *testing.T)
// "2026-10-28[21-15]_-304.45, 24.18, 146.26_0.01327, 0.79807, -0.01759, 0.60216_10.81 (0).png" и тот же с "(1)" → одинаковый Key()
func TestParseNameInvalid(t *testing.T) // "photo.png", "2026-10-28[21-06]_abc.png", real0 без ".png" → false
```

- [ ] **Step 2:** `go test ./internal/shots/ -run Parse` — FAIL.
- [ ] **Step 3: Реализовать `ParseName`** регуляркой из спеки (числа — `strconv.ParseFloat`, точка как разделитель); `Key()` — `fmt.Sprintf("%.2f|%.2f|%.2f|%.5f", x, y, z, qy)`.
- [ ] **Step 4:** PASS.
- [ ] **Step 5: Тесты слежения**

```go
func TestWatcherReportsPng(t *testing.T)        // создать real0 в t.TempDir() → onShot(real0, …) за ≤2 c
func TestWatcherIgnoresOther(t *testing.T)      // создать "x.txt" → вызова нет за 1 c
func TestWatcherCreateThenDelete(t *testing.T)  // создать real0 и сразу удалить → onShot всё равно вызван с real0
```

- [ ] **Step 6: Реализовать `Watcher`**: fsnotify на `dir`, событие `Create` с расширением `.png` (без учёта регистра) → `onShot(filepath.Base(name), time.Now())`; файл не открывать и не stat'ить.
- [ ] **Step 7:** `go test ./internal/shots/` (Linux) и `./scripts/wintest.sh internal/shots` (Windows) — оба PASS.
- [ ] **Step 8: Commit** — «Разбор имён скриншотов и слежение за папкой».

---

### Task 4: Логи игры (`internal/gamelog`)

**Files:**
- Create: `internal/gamelog/parse.go`, `internal/gamelog/tailer.go`, `internal/gamelog/gamelog_test.go`

**Interfaces:**
- Produces:
```go
type Kind int
const (EvMode Kind = iota + 1; EvMap; EvRaidStart)
type Event struct {
    Kind   Kind
    At     time.Time // из префикса строки, локальное время
    Mode   string    // "pve" | "regular" (для EvMode)
    Bundle string    // "maps/shopping_mall.bundle" (для EvMap)
}
func ParseLine(line string) (Event, bool)
func LatestSession(logsDir string) (string, error) // подпапка с самым свежим ModTime
type Tailer struct{ /* текущая сессия, смещения по файлам, хвост неполной строки */ }
func NewTailer(logsDir string, onEvent func(Event)) *Tailer
func (t *Tailer) Poll() error // вызывается раз в секунду
```

- [ ] **Step 1: Тесты разбора** (строки — реальные, из логов владельца)

```go
const (
    lMode  = "2026-10-28 21:02:54.762|1.2.5.0.48146|Info|application|Session mode: Pve"
    lReg   = "2026-10-28 21:02:24.793|1.2.5.0.48146|Info|application|Session mode: Regular"
    lMap   = "2026-10-28 21:04:44.152|1.2.5.0.48146|Info|application|scene preset path:maps/shopping_mall.bundle rcid:Shopping_Mall.ScenesPreset.asset"
    lStart = "2026-10-28 21:06:12.592|1.2.5.0.48146|Info|application|GameStarted:69.1(0) real:89.98(0) diff:20.88"
    lOther = "2026-10-28 21:04:44.152|1.2.5.0.48146|Debug|application|TRACE-NetworkGameMatching G"
)
func TestParseLine(t *testing.T)
// lMode→{EvMode, Mode "pve"}; lReg→{EvMode, "regular"}; lMap→{EvMap, Bundle "maps/shopping_mall.bundle"};
// lStart→{EvRaidStart, At == time.Date(2026,10,28,21,6,12,592e6,time.Local)}; lOther, "", "   at Foo()" → false
```

- [ ] **Step 2:** FAIL → **Step 3: Реализовать `ParseLine`**: время — `time.ParseInLocation("2006-01-02 15:04:05.000", line[:23], time.Local)`; подстроки `Session mode: `, `scene preset path:` (до пробела), `|GameStarted:`. → **Step 4:** PASS.

- [ ] **Step 5: Тесты Tailer**

```go
func TestTailerReadsSessionFromStart(t *testing.T)
// logs/log_a/x application_000.log с lMode, lMap → первый Poll отдаёт [EvMode, EvMap]
func TestTailerPartialLine(t *testing.T)
// дописать lStart без "\n" → Poll ничего; дописать "\n" → Poll отдаёт EvRaidStart; повторный Poll — ничего
func TestTailerSwitchesSession(t *testing.T)
// создать log_b (ModTime позже log_a) с lMap → Poll отдаёт EvMap из log_b
func TestTailerNoLogsDir(t *testing.T) // несуществующий logsDir → Poll возвращает nil, событий нет
```

- [ ] **Step 6: Реализовать Tailer**: в сессии читать все `*application_*.log` (по имени), открывать на каждый `Poll` через `os.Open` (на Windows Go открывает с `FILE_SHARE_READ|FILE_SHARE_WRITE`, игра писать не мешает); новую сессию — с начала, затем по сохранённым смещениям; потреблять только до последнего `\n`; если файл стал короче смещения — читать с начала.
- [ ] **Step 7:** `go test ./internal/gamelog/` и `./scripts/wintest.sh internal/gamelog` — PASS.
- [ ] **Step 8: Commit** — «Чтение логов игры: режим, карта, начало рейда».

---

### Task 5: Поиск папок игры и скриншотов (`internal/paths`)

**Files:**
- Create: `internal/paths/paths.go`, `internal/paths/detect_windows.go`, `internal/paths/detect_other.go`, `internal/paths/paths_test.go`, `internal/paths/detect_windows_test.go`

**Interfaces:**
- Produces:
```go
func ParseLibraryFolders(vdf string) []string
func ChooseGameDir(candidates []string) string // у кого в Logs самая свежая подпапка; "" если ни у кого
func Detect() (gameDir, shotsDir string)       // Windows: реестр + Steam + Known Folder; иначе "", ""
```

- [ ] **Step 1: Тесты**

```go
func TestParseLibraryFolders(t *testing.T)
// вход: "libraryfolders"{ "0"{ "path" "C:\\Program Files (x86)\\Steam" } "1"{ "path" "C:\\SteamLibrary" } } (в формате vdf, с табами)
// → []string{`C:\Program Files (x86)\Steam`, `C:\SteamLibrary`}
func TestChooseGameDir(t *testing.T)
// A/Logs/log_old (ModTime 2025-12-26), B/Logs/log_new (сейчас), C без Logs → B; пустой список → ""
```
`detect_windows_test.go` (`//go:build windows`): `TestDetectThisPC` — пропускается без `TM_REAL=1`; иначе ожидает `gameDir == C:\SteamLibrary\steamapps\common\Escape from Tarkov\build` и существующий `shotsDir`.

- [ ] **Step 2:** FAIL → **Step 3: Реализовать.** Кандидаты и порядок — спека, раздел «Поиск путей». Documents — `windows.KnownFolderPath(windows.FOLDERID_Documents, 0)`; реестр — `golang.org/x/sys/windows/registry`. → **Step 4:** `go test ./internal/paths/` PASS; `TM_REAL=1 ./scripts/wintest.sh internal/paths` PASS (WSLENV должен пробросить `TM_REAL`: `WSLENV=TM_REAL TM_REAL=1 ...`).
- [ ] **Step 5: Commit** — «Автопоиск папки игры и скриншотов».

---

### Task 6: Данные tarkov.dev и кэш (`internal/data`)

**Files:**
- Create: `internal/data/store.go`, `internal/data/store_test.go`, `internal/data/testdata/{maps.json,pve_maps.json,pve_maps_ru.json,Interchange.svg}`

**Interfaces:**
- Consumes: `mapgeo.MapDef`, `mapgeo.Vec3`
- Produces:
```go
var ErrNoData = errors.New("нет данных: нет сети и нет кэша")
type Extract struct{ Name, Faction string; Pos mapgeo.Vec3; Paid, Switch bool } // Faction: "pmc"|"scav"|"shared"
type Transit struct{ Name string; Pos mapgeo.Vec3 }
type GameMap struct{ ID, NormalizedName, Name, ScenePath string; Extracts []Extract; Transits []Transit }
type Store struct {
    JSONBase    string // "https://json.tarkov.dev"
    MapsJSONURL string // "https://raw.githubusercontent.com/the-hideout/tarkov-dev/HEAD/src/data/maps.json"
    SVGBase     string // "" = брать svgPath как есть; иначе заменить префикс "https://assets.tarkov.dev/maps/svg"
    SVGFallback string // "https://raw.githubusercontent.com/the-hideout/tarkov-dev-svg-maps/HEAD"
    // + cacheDir, client, now
}
func NewStore(cacheDir string, client *http.Client, now func() time.Time) *Store
func (s *Store) Load(mode string, force bool) error
func (s *Store) MapByScene(scenePath string) (GameMap, bool)
func (s *Store) MapDef(normalizedName string) (mapgeo.MapDef, bool)
func (s *Store) SVG(def mapgeo.MapDef) (string, error)
func (s *Store) Updated() time.Time
```

- [ ] **Step 1: Фикстуры** — урезанные реальные файлы (из `C:\Users\vadimushka_d\Tools\TarkovMap\spike\data\`): в `maps.json` — записи `interchange` и `factory` (только `maps[]` с `projection == "interactive"`), в `pve_maps.json` — `{"data":{"maps":{<id Развязки>: …}}}` с тремя выходами (`ex_interchange_pmc_saferoom` pmc; `ex_interchange_scav_railway` scav; `ex_interchange_pmc_auto` pmc с непустым `transferItem`) и одним переходом `tr_interchange_customs_DESC`, плюс карта `the-lab` (`scenePath` `maps/laboratory_preset.bundle`, без выходов); в `pve_maps_ru.json` — переводы этих ключей и `"<id> Name"`; `Interchange.svg` — `<svg viewBox="0 0 10 10"><g id="Ground_Level"/></svg>`.

- [ ] **Step 2: Тесты** (httptest-сервер отдаёт фикстуры по путям `/pve/maps`, `/pve/maps_ru`, `/maps.json`, `/svg/Interchange.svg`; счётчик запросов)

```go
func TestLoadAndLookup(t *testing.T)
// MapByScene("maps/shopping_mall.bundle") → Name "Развязка", NormalizedName "interchange";
// выход "Безопасная комната" Faction "pmc"; "Выход на железной дороге" Faction "scav"; "А-Выход у ТЭЦ" Paid == true;
// Transits[0].Name "Переход на Таможню"
func TestMapDef(t *testing.T)        // MapDef("interchange").CoordinateRotation == 180, Layers[0].SVGLayer "First_Floor",
                                     // Extents[0].Bounds[0] == {{120,218},{-222,-327}}; MapDef("night-factory").Key == "factory";
                                     // MapDef("the-lab") → false
func TestLoadUsesCacheWithin24h(t *testing.T)  // второй Load(…, false) при now+1h — 0 новых запросов; при now+25h — запросы есть
func TestLoadOfflineWithCache(t *testing.T)    // сервер закрыт после первой загрузки, now+25h → Load без ошибки, данные на месте
func TestLoadOfflineNoCache(t *testing.T)      // пустой кэш, сервер закрыт → errors.Is(err, ErrNoData)
func TestSVGFallbackAndCache(t *testing.T)     // основной URL 404, запасной 200 → содержимое; второй вызов — 0 запросов
```

- [ ] **Step 3:** FAIL → **Step 4: Реализовать.**
  - `Load`: качать `MapsJSONURL`, `{JSONBase}/{mode}/maps`, `{JSONBase}/{mode}/maps_ru` в `cacheDir` (`maps.json`, `{mode}_maps.json`, `{mode}_maps_ru.json`); свежесть — по `ModTime` файлов, 24 ч; ошибка сети при наличии кэша — не ошибка.
  - Перевод: строковые поля-ключи (`name` карты как `"<id> Name"`, `name` выхода, `description` перехода) → значение из `*_ru`, если нет — ключ.
  - `extents[].bounds` в maps.json — массив вида `[[x,z],[x,z],"имя"]`: брать две первые точки, строку игнорировать.
  - Алиасы `MapDef`: `night-factory→factory`, `ground-zero-21→ground-zero`, `ground-zero-tutorial→ground-zero`. Поддержанной считается запись с непустым `svgPath`.
  - `SVG`: кэш `svg/<файл>`; при ошибке основного URL — `SVGFallback/<файл>`.
- [ ] **Step 5:** `go test ./internal/data/` — PASS.
- [ ] **Step 6: Commit** — «Загрузка и кэш данных tarkov.dev».

---

### Task 7: Логика рейда (`internal/core`) и настройки (`internal/settings`)

**Files:**
- Create: `internal/core/core.go`, `internal/core/core_test.go`, `internal/settings/settings.go`, `internal/settings/settings_test.go`

**Interfaces:**
- Consumes: `gamelog.Event`, `shots.ParseName/Shot`, `data.GameMap`, `mapgeo.*`
- Produces:
```go
type MapSource interface {
    MapByScene(scene string) (data.GameMap, bool)
    MapDef(normalizedName string) (mapgeo.MapDef, bool)
}
type Snapshot struct {
    Mode, MapID, MapName string
    Supported            bool
    RaidStart            time.Time
    LastShot             string // "21:40" или ""
    Message              string // для статусной строки
}
type Marker struct {
    Kind          string  // "extract" | "transit" | "shot"
    Name, Faction string
    Paid, Switch  bool
    PX, PY        float64
    Layer         string
    DirX, DirY    float64 // только shot: единичный вектор взгляда в координатах карты
    Time          string  // только shot
}
type LayerInfo struct{ ID, Name string } // ID = svgLayer; первым — базовый слой с Name ""
type MapView struct {
    MapID, Name string
    Supported   bool
    Rect        mapgeo.Rect
    Layers      []LayerInfo
    Markers     []Marker // выходы и переходы, затем точки маршрута по времени
}
func New(src MapSource) *Core
func (c *Core) OnLogEvent(e gamelog.Event)
func (c *Core) OnShot(name string, at time.Time) bool // true, если маршрут изменился
func (c *Core) SetSource(src MapSource)               // после смены режима данных; пересчитать текущую карту
func (c *Core) Snapshot() Snapshot
func (c *Core) View() MapView

// settings
type WindowPos struct{ X, Y, W, H int; Maximised bool }
type Settings struct {
    GameDir, ShotsDir string          // ручные пути; "" = авто
    Side              string          // "pmc" | "scav", по умолчанию "pmc"
    Filters           map[string]bool // ключи — Task 8
    Window            *WindowPos
}
func Load(dir string) Settings          // нет файла/битый → значения по умолчанию
func Save(dir string, s Settings) error // dir/settings.json
```

Правила `Core` (решения по спеке): `EvMode` — запомнить режим; `EvMap` — найти карту по `Bundle`, очистить маршрут и `RaidStart`; `EvRaidStart` — `RaidStart = e.At`; `OnShot` — игнорировать, если имя не разобрано, карта не поддержана, `RaidStart` пуст или `at` раньше `RaidStart`, либо `Key()` уже есть. Сообщение для неподдержанной карты: `Карта «<Name>» пока не поддерживается`. `DirX/DirY` — нормированная разность `Project(x+fx, z+fz) − Project(x, z)`.

- [ ] **Step 1: Тесты** (фейковый `MapSource` с Развязкой из Task 2/6 и картой «Лаборатория» без `MapDef`)

```go
func TestRaidFlow(t *testing.T)
// EvMode pve, EvMap shopping_mall (21:04:44), EvRaidStart (21:06:12), OnShot(real0, 21:06:30) → true;
// View(): 1 shot-маркер, Layer "Ground_Level", PX/PY == mapgeo.Project(ic, -312.74, -111.95); Snapshot.LastShot "21:06"
func TestDuplicateShot(t *testing.T)       // тот же скрин с "(1)" → false, маркер один
func TestShotBeforeRaidIgnored(t *testing.T) // OnShot до EvRaidStart и с at < RaidStart → false
func TestNewMapClearsRoute(t *testing.T)   // новый EvMap → в View нет shot-маркеров
func TestUnsupportedMap(t *testing.T)      // EvMap "maps/laboratory_preset.bundle" → Supported false, Message "Карта «Лаборатория» пока не поддерживается"; OnShot → false
func TestShotDirection(t *testing.T)       // скрин с Q (0,0.70711,0,0.70711) на Развязке → DirX ≈ -1, DirY ≈ 0 (±1e-3)
func TestRestoreMidRaid(t *testing.T)      // реплей событий текущей сессии + OnShot с at = ModTime старых файлов → маршрут восстановлен только из скринов после RaidStart
```
`settings_test.go`: `TestSettingsRoundTrip` (Save→Load равны), `TestSettingsDefaults` (нет файла → `Side == "pmc"`, `Filters` не nil).

- [ ] **Step 2:** FAIL → **Step 3: Реализовать** → **Step 4:** `go test ./internal/core/ ./internal/settings/` PASS.
- [ ] **Step 5: Commit** — «Логика рейда и настройки».

---

### Task 8: Обвязка Wails и фронтенд карты

**Files:**
- Modify: `main.go`, `app.go`
- Create: `frontend/dist/index.html`, `frontend/dist/style.css`, `frontend/dist/app.js`, `frontend/dist/map.js`, `scripts/smoke.ps1`
- Delete: `Ping()` из Task 1

**Interfaces:**
- Consumes: всё из Task 3–7.
- Produces (методы для фронтенда, `window.go.main.App.*`):
```go
func (a *App) GetSnapshot() core.Snapshot
func (a *App) GetView() core.MapView
func (a *App) GetSVG(mapID string) (string, error)
func (a *App) GetSettings() settings.Settings
func (a *App) SetSide(side string) error
func (a *App) SetFilters(f map[string]bool) error
func (a *App) SetPaths(gameDir, shotsDir string) error // "" = авто; перезапускает слежение
func (a *App) RefreshData() error
func (a *App) GetPaths() (gameDir, shotsDir string)
func (a *App) GetDataUpdated() time.Time // Store.Updated()
```
События Wails: `"snapshot"` (payload `core.Snapshot`) и `"view"` (без payload — фронтенд вызывает `GetView()`).
Флаги командной строки (для смоук-теста, не сохраняются): `-game <dir>`, `-shots <dir>`.

- [ ] **Step 1: `app.go`**. `startup`: настройки → пути (флаги > настройки > `paths.Detect()`) → `data.Store.Load(режим, false)` (режим по умолчанию `"pve"`, до первой строки `Session mode`) → `core.New` → `Tailer` над `<game>\Logs` (первый `Poll` сразу: реплей текущей сессии) → реплей скринов: файлы `*.png` в `shotsDir` по `ModTime`, `OnShot(name, ModTime)` → `shots.Watcher` → тикер 1 с: `Poll`. При `EvMode` с другим режимом — `Store.Load(mode,false)` и `core.SetSource`. После любого изменения — `runtime.EventsEmit("snapshot", …)` и `"view"`. Ошибку `ErrNoData` показать через `Snapshot.Message`: «Нет данных карт: нет сети и нет кэша. Подключитесь к интернету и нажмите «Обновить данные».»

- [ ] **Step 2: Фронтенд** — перенести рендер из спайка (`C:\Users\vadimushka_d\Tools\TarkovMap\spike\template.html`), но без пересчёта координат в JS: SVG вставляется в прямоугольник `view.Rect`, маркеры рисуются по `PX/PY`, конус — по `DirX/DirY`.
  - Фильтр (ключи `Settings.Filters`, по умолчанию все `true`, кроме `extract_other`): `route` — маршрут; `extract_own` — выходы моей стороны (`Faction == Side`); `extract_shared`; `extract_other` — выходы другой стороны; `transit`. Переключатель стороны «ЧВК / Дикий» → `SetSide`.
  - Этажи: радиокнопки по `view.Layers` (подпись: базовый — «Основной», `"Nth Floor"` → «N-й этаж», `Underground*` → «Подземный уровень», `Basement` → «Подвал», `Bunkers` → «Бункеры», иначе как есть). При новом скрине этаж переключается на `Layer` последней точки; маркеры других этажей — прозрачность 0.35.
  - Выходы: подпись — название; `Paid` → значок «₽», `Switch` → значок рубильника; в подсказке — сторона («ЧВК», «Дикие», «Совместный») и условия словами.
  - Кнопка «Центр на мне» — центр вида на последней точке маршрута.
  - Статусная строка: `PvE`/`PvP`, название карты, «последний скрин 21:40», пути найдены/не найдены, дата данных, `Snapshot.Message`. Неподдержанная карта — сообщение вместо карты.
  - Настройки (диалог): пути к игре и скринам (пусто = авто) → `SetPaths`; «Обновить данные» → `RefreshData`; «О программе»: «Карты: the-hideout / tarkov.dev, CC BY-NC-SA 4.0. Данные: tarkov.dev».
  - Масштаб колесом и перетаскивание — как в спайке.

- [ ] **Step 3: `scripts/smoke.ps1`** — создаёт `%TEMP%\tm-smoke\build\Logs\log_smoke\smoke application_000.log` и `%TEMP%\tm-smoke\shots`, запускает `build\bin\TarkovMap.exe -game %TEMP%\tm-smoke\build -shots %TEMP%\tm-smoke\shots`, через 3 с дописывает `lMode`, `lMap`, `lStart` (с текущими датой и временем), затем раз в 2 с кладёт 3 пустых png с именами реальных скринов (21-06, 21-36 у «Безопасной комнаты», 21-40), дату/время в имени — текущие.

- [ ] **Step 4: Проверка в Windows**
Run: `./scripts/build.sh` (WSL), затем `powershell -File scripts\smoke.ps1`.
Expected: окно открывается на Развязке, статус `PvE · Развязка`; появляются 3 точки с конусами, вторая — у выхода «Безопасная комната»; этаж «Основной»; фильтры скрывают и показывают слои; после перезапуска exe с теми же флагами маршрут восстанавливается. Сделать скриншот.

- [ ] **Step 5: Проверка на реальной игре** — запустить exe без флагов, в Windows: статус показывает найденные пути `C:\SteamLibrary\…\build` и `…\Screenshots`. Окно не должно падать при закрытой игре.
- [ ] **Step 6: Commit** — «Окно карты: маршрут, выходы, фильтр, этажи».

---

### Task 9: Положение окна и итоговая проверка

**Files:**
- Modify: `main.go`, `app.go`

**Interfaces:**
- Consumes: `settings.WindowPos`, Wails `runtime.WindowGetPosition/WindowGetSize/WindowIsMaximised/WindowSetPosition`
- Produces: `func (a *App) beforeClose(ctx context.Context) bool` (сохраняет `Settings.Window`, возвращает `false`).

- [ ] **Step 1:** `main.go`: `OnBeforeClose: app.beforeClose`, `Width/Height` из `Settings.Window`; в `OnDomReady` — `WindowSetPosition` и `WindowMaximise` при `Maximised`.
- [ ] **Step 2: Проверка на втором мониторе**: перетащить окно на второй монитор, изменить размер, закрыть, открыть. Expected: окно открылось там же и того же размера.
  Если Wails ставит позицию относительно текущего монитора и окно оказывается не там — сохранять/восстанавливать через Win32 (`GetWindowRect`/`SetWindowPos` по HWND окна, найденному `FindWindowW` по заголовку «Tarkov Map», через `golang.org/x/sys/windows`), повторить проверку.
- [ ] **Step 3:** Полный прогон: `go test ./...` (WSL) — PASS; `./scripts/wintest.sh` для `internal/shots`, `internal/gamelog`, `internal/paths` — PASS; `smoke.ps1` — как в Task 8.
- [ ] **Step 4: Commit** — «Запоминание положения окна»; `git push`.
