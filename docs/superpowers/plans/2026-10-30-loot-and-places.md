# Лут и места — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Новая вкладка ленты «Лут и места»: названия мест, контейнеры по 7 группам, лут на земле с поиском по названию, замки, рубильники, опасные зоны, пулемёты и боссы — всё по флажкам. Плюс иконка приложения.

**Исполнение:** владелец сказал выполнять план сразу, без согласования, и показать в конце готовое приложение (30.10) — inline (superpowers:executing-plans), слияние в `main` по-прежнему только с его согласия.

**Architecture:** Go разбирает новые поля из уже скачиваемых `maps.json` и `{mode}_maps.json` и необязательный новый файл `{mode}/items_ru`, проецирует точки и определяет этажи (как у выходов) и отдаёт фронтенду отдельным вызовом `GetMapContent` — один раз на карту и после обновления данных (по `ContentGen` в виде). Фронтенд рисует слои в пикселях экрана, строит DOM группы только когда она включена, поиск по луту идёт в JS.

**Tech Stack:** Go 1.x + Wails v2.17, чистые HTML/JS/SVG без сборщика.

**Spec:** `docs/superpowers/specs/2026-10-29-loot-and-places-design.md` (основной дизайн — `docs/superpowers/specs/2026-10-28-tarkov-map-design.md`).

## Global Constraints

- Сеть — только GET к `json.tarkov.dev`, `assets.tarkov.dev`, `raw.githubusercontent.com`. Новый адрес: `https://json.tarkov.dev/{mode}/items_ru`.
- Файл `items_ru` необязателен: без него карта, выходы и маршрут работают как прежде, недоступен только поиск.
- По умолчанию включены только «Названия мест»; контейнеры, «Все точки», всё из «Прочего» — выключены.
- Интерфейс по-русски. Поиск — без учёта регистра, `ё` = `е`, подстрока в `Name` или `ShortName` из `items_ru`, от 2 символов, задержка ~150 мс.
- Колесо и перетаскивание на Улицах Таркова со всеми слоями (≈2500 значков) — без заметных рывков.
- Приложение не трогает процесс и память игры, без оверлеев; скриншоты не удаляет.
- Работа в ветке `loot-and-places`; слияние в `main` — только с согласия владельца. Коммиты по-русски, в конце `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`; почта автора `vadimushka.d@gmail.com`.
- Go-тесты — в WSL: `MSYS_NO_PATHCONV=1 wsl -e bash -lc 'cd /mnt/c/Users/vadimushka_d/Projects/tarkov-map && CGO_ENABLED=1 go test -race ./...'`. Автотестов фронтенда в проекте нет: фронтенд проверяется в браузерной панели на реальных данных (Задача 6 готовит данные и заглушку).
- Окно приложения не выводить на передний план, если запущена игра (процесс `EscapeFromTarkov`).

## Review Focus

1. **Фоновое обновление данных при открытой карте** — слои пересобираются по новым данным, флажки и строка поиска сохраняются (Go: `TestContentGenGrowsOnSetSource`, Задача 5; браузер: шаг в Задаче 7).
2. **Карта без меток** (Терминал, обучающий Эпицентр: нет подписей и контейнеров) — нули в ленте, пустые слои, без ошибок в консоли (Go: `TestContentEmptyForMapWithoutContent`, Задача 5; браузер: шаг в Задаче 7).
3. **Края поиска**: 1 символ, ничего не найдено, `ё`, очистка крестиком (браузер, Задача 8).
4. **Этажи**: подписи магазинов Развязки видны только на своём этаже, подписи земли — всегда; значки и контуры чужого этажа приглушены (браузер, Задача 7).
5. **Новый скрин и смена стороны** перерисовывают выходы и маршрут, но не пересобирают и не скрывают слои лута (браузер, Задача 7).

---

### Task 1: Этаж подписи по диапазону высот

**Files:**
- Modify: `internal/mapgeo/geo.go` (рядом с `LayerOf`)
- Test: `internal/mapgeo/geo_test.go`

**Interfaces:**
- Produces: `func LayerOfSpan(md MapDef, x, z, bottom, top float64) string` — SVG-слой этажа, в один `Extent` которого диапазон `[bottom, top]` укладывается целиком (границы **включительно**) и в чьи `Bounds` попадает точка (нет `Bounds` — подходит любая точка); иначе `md.SVGLayer`.

- [ ] **Step 1: Написать падающий тест**

```go
func TestLayerOfSpan(t *testing.T) {
	mall := [][2][2]float64{{{120, 218}, {-222, -327}}}
	ic := MapDef{SVGLayer: "Ground_Level", Layers: []Layer{
		{Name: "2nd Floor", SVGLayer: "First_Floor", Extents: []Extent{{Height: [2]float64{25, 34}, Bounds: mall}}},
		{Name: "3rd Floor", SVGLayer: "Second_Floor", Extents: []Extent{{Height: [2]float64{34, 1000}, Bounds: mall}}},
	}}
	factory := MapDef{SVGLayer: "Ground_Floor", Layers: []Layer{
		{Name: "3rd Floor", SVGLayer: "Third_Floor", Extents: []Extent{{Height: [2]float64{6, 10000}}}},
		{Name: "Underground", SVGLayer: "Underground_Level", Extents: []Extent{{Height: [2]float64{-1000, 0.5}}}},
	}}
	cases := []struct {
		name             string
		md               MapDef
		x, z, bottom, top float64
		want             string
	}{
		{"магазин 2-го этажа ТЦ", ic, -34, -235, 25, 33, "First_Floor"},
		{"магазин 3-го этажа ТЦ", ic, -34, -235, 34, 999, "Second_Floor"},
		{"земля", ic, -34, -235, -1, 10, "Ground_Level"},
		{"вне прямоугольника этажа", ic, 300, 300, 25, 33, "Ground_Level"},
		{"верх включительно", factory, 0, 0, 6, 10000, "Third_Floor"},
		{"диапазон шире любого этажа (санаторий Берега)", factory, 0, 0, -100, 100, "Ground_Floor"},
	}
	for _, c := range cases {
		if got := LayerOfSpan(c.md, c.x, c.z, c.bottom, c.top); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Запустить — должен упасть**

Run: `MSYS_NO_PATHCONV=1 wsl -e bash -lc 'cd /mnt/c/Users/vadimushka_d/Projects/tarkov-map && go test ./internal/mapgeo/ -run TestLayerOfSpan'`
Expected: FAIL, `undefined: LayerOfSpan`

- [ ] **Step 3: Реализовать `LayerOfSpan` в `internal/mapgeo/geo.go`** (переиспользовать `inside`).

- [ ] **Step 4: Запустить — должен пройти**

Run: та же команда, затем `go test ./internal/mapgeo/`
Expected: PASS

- [ ] **Step 5: Коммит** — `git add internal/mapgeo && git commit` с сообщением «Этаж подписи по диапазону высот».

---

### Task 2: Метки карты из `{mode}_maps.json`

**Files:**
- Create: `internal/data/content.go` (типы меток, группы контейнеров, разбор)
- Modify: `internal/data/store.go` (`GameMap`, `rawMap`, `parseMaps`)
- Modify: `internal/data/testdata/pve_maps.json`, `internal/data/testdata/pve_maps_ru.json`
- Test: `internal/data/content_test.go`

**Interfaces:**
- Produces (пакет `data`):

```go
type Container struct {
	Group, Name string // Group: weapon|med|tech|safe|cache|body|misc
	Pos         mapgeo.Vec3
}
type Loose struct {
	Pos   mapgeo.Vec3
	Items []string // id предметов
}
type Lock struct {
	Kind       string // lockType: door|trunk|…
	KeyID      string
	NeedsPower bool
	Pos        mapgeo.Vec3
}
type Switch struct {
	Name string
	Pos  mapgeo.Vec3
}
type Hazard struct {
	Kind    string // hazardType: minefield|sniper|hazard|…
	Pos     mapgeo.Vec3
	Outline []mapgeo.Vec3
}
type Weapon struct {
	Name string
	Pos  mapgeo.Vec3
}
type BossSpot struct {
	Chance float64 // spawnLocations[].chance
	Pos    mapgeo.Vec3
}
type Boss struct {
	Name   string
	Chance float64 // spawnChance
	Spots  []BossSpot
}
func containerGroup(normalizedName string) string
```

- `GameMap` получает поля `Containers []Container; Loose []Loose; Locks []Lock; Switches []Switch; Hazards []Hazard; Weapons []Weapon; Bosses []Boss`.
- Названия по-русски через уже существующий `loadTranslations`:
  - контейнер — `tr(data.lootContainers[id].name)`, группа — по `normalizedName`; id нет в словаре — `Name = id`, группа `misc`;
  - рубильник — `tr(name)`;
  - пулемёт — `tr(data.stationaryWeapons[id].name)`;
  - босс — `tr(data.mobs[mob].name)`; в `Bosses` только `mob` с префиксом `boss`; `Spots` — по одной на каждую позицию каждого `spawnLocation`, зоны без `positions` ничего не дают.
- Группы — таблица из спеки: weapon = `weapon-box`, `grenade-box`, `wooden-ammo-box`; med = `medbag`, `medcase`, `medical-supply-crate`; tech = `pc-block`, `toolbox`, `technical-supply-crate`; safe = `safe`, `bank-safe`, `cash-register`, `bank-cash-register`; cache = `buried-barrel-cache`, `ground-cache`, `shturmans-stash`; body = `dead-scav`, `scav-body`, `pmc-body`, `civilian-body`, `lab-technician-body`; всё прочее и незнакомое — misc.

- [ ] **Step 1: Дополнить фикстуры**

В `testdata/pve_maps.json` у Развязки (`5714dbc024597771384a510d`) добавить:

```json
"lootContainers": [
  {"lootContainer": "c_weapon", "position": {"x": 10, "y": 1, "z": 20}},
  {"lootContainer": "c_new", "position": {"x": 11, "y": 1, "z": 21}}
],
"lootLoose": [{"position": {"x": 1, "y": 30, "z": 2}, "items": ["i_flash", "i_led"]}],
"locks": [{"lockType": "door", "key": "i_key", "needsPower": true, "position": {"x": 3, "y": 1, "z": 4}}],
"switches": [{"name": "switch_mall", "position": {"x": 5, "y": 1, "z": 6}}],
"hazards": [{"hazardType": "minefield", "name": "DamageType_Landmine", "position": {"x": 7, "y": 1, "z": 8},
  "outline": [{"x": 0, "y": 1, "z": 0}, {"x": 10, "y": 1, "z": 0}, {"x": 10, "y": 1, "z": 10}]}],
"stationaryWeapons": [{"stationaryWeapon": "w_utes", "position": {"x": 9, "y": 1, "z": 9}}],
"bosses": [
  {"mob": "bossKilla", "spawnChance": 0.75, "spawnLocations": [
    {"chance": 0.33, "positions": [{"x": 1, "y": 1, "z": 1}, {"x": 2, "y": 1, "z": 2}]},
    {"chance": 0.5, "positions": []}]},
  {"mob": "pmcBEAR", "spawnChance": 1, "spawnLocations": [{"chance": 1, "positions": [{"x": 3, "y": 1, "z": 3}]}]}
]
```

В `data` (рядом с `maps`):

```json
"lootContainers": {
  "c_weapon": {"id": "c_weapon", "name": "c_weapon Name", "normalizedName": "weapon-box"},
  "c_new": {"id": "c_new", "name": "c_new Name", "normalizedName": "mystery-crate"}
},
"stationaryWeapons": {"w_utes": {"id": "w_utes", "name": "w_utes Name"}},
"mobs": {"bossKilla": {"id": "bossKilla", "name": "bossKilla"}, "pmcBEAR": {"id": "pmcBEAR", "name": "pmcBEAR"}}
```

В `testdata/pve_maps_ru.json`: `"c_weapon Name": "Оружейный кофр"`, `"c_new Name": "Таинственный ящик"`, `"switch_mall": "Главный выключатель питания ТЦ"`, `"w_utes Name": "Станковый пулемет НСВ \"Утес\" 12.7x108"`, `"bossKilla": "Килла"`.

- [ ] **Step 2: Написать падающие тесты** в `internal/data/content_test.go`

```go
func TestLoadMapContent(t *testing.T) {
	s, _, _, _ := loaded(t)
	m, _ := s.MapByScene("maps/shopping_mall.bundle")
	if len(m.Containers) != 2 || m.Containers[0] != (Container{Group: "weapon", Name: "Оружейный кофр", Pos: mapgeo.Vec3{X: 10, Y: 1, Z: 20}}) {
		t.Errorf("Containers = %+v", m.Containers)
	}
	if m.Containers[1].Group != "misc" || m.Containers[1].Name != "Таинственный ящик" {
		t.Errorf("незнакомый тип: %+v", m.Containers[1])
	}
	if len(m.Loose) != 1 || strings.Join(m.Loose[0].Items, ",") != "i_flash,i_led" {
		t.Errorf("Loose = %+v", m.Loose)
	}
	if len(m.Locks) != 1 || m.Locks[0].Kind != "door" || m.Locks[0].KeyID != "i_key" || !m.Locks[0].NeedsPower {
		t.Errorf("Locks = %+v", m.Locks)
	}
	if len(m.Switches) != 1 || m.Switches[0].Name != "Главный выключатель питания ТЦ" {
		t.Errorf("Switches = %+v", m.Switches)
	}
	if len(m.Hazards) != 1 || m.Hazards[0].Kind != "minefield" || len(m.Hazards[0].Outline) != 3 {
		t.Errorf("Hazards = %+v", m.Hazards)
	}
	if len(m.Weapons) != 1 || !strings.Contains(m.Weapons[0].Name, "Утес") {
		t.Errorf("Weapons = %+v", m.Weapons)
	}
	if len(m.Bosses) != 1 || m.Bosses[0].Name != "Килла" || m.Bosses[0].Chance != 0.75 ||
		len(m.Bosses[0].Spots) != 2 || m.Bosses[0].Spots[0].Chance != 0.33 {
		t.Errorf("Bosses = %+v", m.Bosses)
	}
}

func TestContainerGroup(t *testing.T) {
	cases := map[string]string{
		"weapon-box": "weapon", "grenade-box": "weapon", "wooden-ammo-box": "weapon",
		"medbag": "med", "medcase": "med", "medical-supply-crate": "med",
		"pc-block": "tech", "toolbox": "tech", "technical-supply-crate": "tech",
		"safe": "safe", "bank-safe": "safe", "cash-register": "safe", "bank-cash-register": "safe",
		"buried-barrel-cache": "cache", "ground-cache": "cache", "shturmans-stash": "cache",
		"dead-scav": "body", "scav-body": "body", "pmc-body": "body", "civilian-body": "body", "lab-technician-body": "body",
		"duffle-bag": "misc", "drawer": "misc", "jacket": "misc", "something-new": "misc",
	}
	for n, want := range cases {
		if got := containerGroup(n); got != want {
			t.Errorf("containerGroup(%q) = %q, want %q", n, got, want)
		}
	}
}
```

- [ ] **Step 3: Запустить — должен упасть**

Run: `... go test ./internal/data/ -run 'TestLoadMapContent|TestContainerGroup'`
Expected: FAIL при компиляции: нет полей `Containers`… и функции `containerGroup`.

- [ ] **Step 4: Реализовать** типы и `containerGroup` в `content.go`; в `store.go` расширить `rawMap` новыми полями, распарсить верхнеуровневые `data.lootContainers`, `data.stationaryWeapons`, `data.mobs` (словари id → `{name, normalizedName}`) и заполнить новые поля `GameMap` в `parseMaps`. Сигнатура `parseMaps(mapsFile, ruFile string)` не меняется.

- [ ] **Step 5: Запустить пакет целиком — должно пройти**

Run: `... go test -race ./internal/data/`
Expected: PASS (старые тесты тоже)

- [ ] **Step 6: Коммит** — «Контейнеры, лут, замки, рубильники, опасные зоны, пулемёты и боссы из данных карт».

---

### Task 3: Названия мест из `maps.json`

**Files:**
- Create: `internal/data/places.go` (словарь и `placeName`)
- Modify: `internal/data/store.go` (`rawDef.Labels`, `parseDefs`, поле `labels` в `Store`, метод `Labels`)
- Modify: `internal/data/real_test.go` (новая сигнатура `parseDefs`)
- Modify: `internal/data/testdata/maps.json`
- Test: `internal/data/places_test.go`

**Interfaces:**
- Produces:

```go
type Label struct {
	Text        string   // по-русски, если есть в словаре
	X, Z        float64
	Bottom, Top *float64 // нет в данных — nil
}
func (s *Store) Labels(normalizedName string) []Label // с учётом aliases, как MapDef
func placeName(text string) string
func parseDefs(file string) (map[string]mapgeo.MapDef, map[string][]Label, error)
```

- Подписи берутся из того же варианта карты, что и `MapDef` (первый `interactive` с `svgPath`). `position` — `[x, z]`; подпись с `position` короче двух чисел пропускается. `rotation` и `size` не используются.
- `placeName`: сначала словарь `placeRU`, затем префиксы `placePrefixRU`; иначе текст как есть.

Словарь (копировать как есть):

```go
// placeRU — русские названия общеизвестных мест; магазины и бренды остаются как в игре.
var placeRU = map[string]string{
	// общие для нескольких карт
	"Power Station": "Электростанция", "Gas Station": "Заправка", "Construction": "Стройка",
	"Cabins": "Домики", "Cottages": "Коттеджи", "Bus Stop": "Остановка", "Crash Site": "Место крушения",
	"Scav Camp": "Лагерь Диких", "Office": "Офис", "Garage": "Гараж",
	// Таможня
	"Big Red": "Красный склад", "Dorms": "Общаги", "New Gas": "Новая заправка", "Old Gas": "Старая заправка",
	"Fortress": "Крепость", "Crackhouse": "Притон", "Streamer House": "Дом стримера", "Main Bridge": "Главный мост",
	"Sniper Hill": "Снайперский холм", "Storage": "Склад", "Trailer Park": "Трейлерный парк",
	"Junk Bridge": "Мусорный мост", "Repair Shop": "Мастерская", "Sniper Ridge": "Снайперский гребень",
	"Old Construction": "Старая стройка", "Skeleton": "Скелет", "Depot": "Депо",
	"Military Checkpoint": "Военный КПП", "Bus Station": "Автостанция", "Powerline Tower": "Опора ЛЭП",
	// Завод
	"Office Building": "Офисы", "Main Office": "Главный офис", "Breach Room": "Комната с проломом",
	"Locked Office": "Закрытый офис", "North Stairs": "Северная лестница", "South Stairs": "Южная лестница",
	"Sky Bridge": "Мостик", "Connector": "Переход", "Locker Rooms": "Раздевалки", "Sinks": "Умывальники",
	"Hole": "Дыра", "Platform": "Платформа", "Servers": "Серверная", "Med Tent": "Медпалатка",
	"Silos": "Силосы", "Heli Crash": "Вертолёт", "Pit": "Яма", "Underground Stash": "Подземный схрон",
	"Scav Bunker": "Бункер Диких", "Blue Containers": "Синие контейнеры", "Wood Room": "Деревянная комната",
	"Glass Hall": "Стеклянный зал", "Boilers": "Котельная", "Pumping Station": "Насосная",
	"East Halls": "Восточные цеха", "Forklifts": "Погрузчики", "Rafters": "Балки",
	// Развязка
	"Go Kart": "Картинг", "Cargo Containers": "Контейнеры", "Wall Break": "Пролом в стене", "Ramps": "Пандусы",
	"Oli Tower": "Башня OLI", "Idea Tower": "Башня IDEA", "Highway Construction": "Стройка трассы",
	// Маяк
	"Train Yard": "Ж/д депо", "Drug Lab": "Нарколаборатория", "Water Treatment": "Очистные сооружения",
	"Pipes": "Трубы", "Gunner Nest": "Пулемётное гнездо", "Island": "Остров", "Convenience": "Магазин",
	"Hillside": "Склон", "Boathouses": "Лодочные сараи", "Dead Tree": "Сухое дерево",
	"Grand Chalet": "Шале", "Tennis Court": "Теннисный корт", "Lightkeeper Island": "Остров Смотрителя",
	// Резерв
	"K Buildings": "Корпуса К", "White Queen": "Белый ферзь", "White Pawn": "Белая пешка",
	"Black Bishop": "Чёрный слон", "White Bishop": "Белый слон", "White King": "Белый король",
	"Black Knight": "Чёрный конь", "White Knight": "Белый конь", "White Rook": "Белая ладья",
	"Black Pawn": "Чёрная пешка", "Train Station": "Вокзал", "Barracks": "Казармы",
	"Mechanic": "Мастерская", "Shipping Yard": "Погрузочная площадка", "Dome": "Купол", "Tarmac": "Аэродром",
	// Берег
	"Resort": "Санаторий", "Weather Station": "Метеостанция", "Radio Tower": "Радиовышка", "Swamp": "Болото",
	"Village": "Деревня", "Tank Bridge": "Танковый мост", "Pier": "Пирс", "Scav Island": "Остров Диких",
	"Bunker": "Бункер", "Crane": "Кран", "Scav Farm": "Ферма Диких", "West Wing": "Западное крыло",
	"East Wing": "Восточное крыло", "Admin": "Администрация",
	// Лес
	"Sawmill": "Лесопилка", "Scav Town": "Посёлок Диких", "Old Sawmill": "Старая лесопилка",
	"Cultist Village": "Деревня сектантов", "USEC Camp": "Лагерь USEC", "Military Camp": "Военный лагерь",
	"Ponds": "Пруды", "Checkpoint": "КПП", "Shack": "Хижина", "Lumber": "Лесосклад",
	"Jaeger's Camp": "Лагерь Егеря", "Sniper Rock": "Снайперская скала", "Convoy": "Колонна",
	// Улицы Таркова
	"Primorsky Ave.": "Приморский пр.", "Kilmov St.": "ул. Климова", "Nikitskaya St.": "ул. Никитская",
	"Verhnyaya St.": "ул. Верхняя", "Malevecha St.": "ул. Малевича", "Chekannaya St.": "ул. Чеканная",
	"Nizhnyaya St.": "ул. Нижняя", "Razvedchikov St.": "ул. Разведчиков", "Zmejskij Alley": "Змейский пер.",
	"Kamchatskaya St.": "ул. Камчатская", "Kilmov Shopping Mall": "ТЦ Климов", "Cinema": "Кинотеатр",
	"Shestyorochka": "Шестёрочка", "Post Office": "Почта", "Corner Restaurant": "Угловой ресторан",
	"School": "Школа", "Vet Clinic": "Ветклиника", "Abandon Factory": "Заброшенный завод", "Diner": "Закусочная",
	"Primorskij 49": "Приморский 49",
	// Эпицентр
	"Science Office": "Научный офис",
}

// placePrefixRU — нумерованные места: «Warehouse 17» → «Склад 17».
var placePrefixRU = [][2]string{{"Warehouse ", "Склад "}, {"Garage ", "Гараж "}, {"Plant ", "Цех "}, {"Pharmacy ", "Аптека "}}
```

- [ ] **Step 1: Дополнить фикстуру** `testdata/maps.json` — у Развязки в объекте карты:

```json
"labels": [
  {"position": [-34, -235], "text": "IDEA", "top": 33, "bottom": 25},
  {"position": [-186, -319], "text": "Power Station", "top": 10, "bottom": -1, "size": 80},
  {"position": [300, 300], "text": "Garage A"},
  {"position": [1], "text": "Битая"}
]
```

- [ ] **Step 2: Написать падающие тесты** в `places_test.go`

```go
func TestLabels(t *testing.T) {
	s, _, _, _ := loaded(t)
	ls := s.Labels("interchange")
	if len(ls) != 3 {
		t.Fatalf("Labels = %+v", ls)
	}
	byText := map[string]Label{}
	for _, l := range ls {
		byText[l.Text] = l
	}
	ps, ok := byText["Электростанция"]
	if !ok || ps.X != -186 || ps.Z != -319 || ps.Bottom == nil || *ps.Bottom != -1 || ps.Top == nil || *ps.Top != 10 {
		t.Errorf("Электростанция: %+v", ps)
	}
	if _, ok := byText["IDEA"]; !ok {
		t.Error("IDEA должна остаться как есть")
	}
	if g, ok := byText["Гараж A"]; !ok || g.Bottom != nil || g.Top != nil {
		t.Errorf("Гараж A: %+v", g)
	}
	if len(s.Labels("factory")) != 0 {
		t.Error("у Завода в фикстуре подписей нет")
	}
}

func TestPlaceName(t *testing.T) {
	cases := map[string]string{"Dorms": "Общаги", "Warehouse 17": "Склад 17", "Pharmacy 2": "Аптека 2", "Kiba": "Kiba", "Garage": "Гараж"}
	for in, want := range cases {
		if got := placeName(in); got != want {
			t.Errorf("placeName(%q) = %q, want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 3: Запустить — должен упасть**

Run: `... go test ./internal/data/ -run 'TestLabels|TestPlaceName'`
Expected: FAIL при компиляции: нет `Labels`, `Label`, `placeName`.

- [ ] **Step 4: Реализовать** `places.go`, разбор подписей в `parseDefs`, хранение в `Store.labels` (под тем же `mu`, присваивается в `parse` вместе с `defs`), метод `Labels`; поправить вызов в `real_test.go`.

- [ ] **Step 5: Запустить пакет — должно пройти**

Run: `... go test -race ./internal/data/`
Expected: PASS

- [ ] **Step 6: Коммит** — «Названия мест из maps.json и словарь русских названий».

---

### Task 4: Названия предметов — необязательный `items_ru`

**Files:**
- Modify: `internal/data/store.go` (`itemsFile`, `Load`, `Fetch`, `Stale`, `parse`, поля `items`, `itemsFailed`)
- Modify: `internal/data/content.go` (`ItemName`, `parseItems`)
- Create: `internal/data/testdata/pve_items_ru.json`
- Modify: `internal/data/store_test.go` (`fakeServer`: путь `/pve/items_ru`, поле `noItems bool` → 404)
- Test: `internal/data/content_test.go`

**Interfaces:**
- Produces:

```go
type ItemName struct{ Name, Short string }
func (s *Store) ItemName(id string) (ItemName, bool)
func parseItems(file string) map[string]ItemName // нет файла или он битый — пустой словарь
```

- Файл: `{JSONBase}/{mode}/items_ru` → `{mode}_items_ru.json`. В `data` берутся только ключи `"<id> Name"` и `"<id> ShortName"`.
- **Не входит в `files(mode)`**: `LoadCached` и `Use` от него не зависят.
- `Load`: после обязательных файлов — `ensure` для `items_ru` без учёта ошибки.
- `Fetch`: после обязательных файлов — попытка скачать `items_ru`; ошибка не возвращается, а запоминается в `itemsFailed[mode]` (успех — сбрасывает).
- `Stale(mode)`: прежнее условие **или** файла `items_ru` нет / он старше суток — но только если `itemsFailed[mode]` не стоит (иначе при недоступном `items_ru` каждая загрузка карты заново качала бы 11 МБ).
- `parse`: `s.items = parseItems(...)` вместе с остальным.

- [ ] **Step 1: Фикстура** `testdata/pve_items_ru.json`:

```json
{"data": {
  "i_flash Name": "Защищённая флешка", "i_flash ShortName": "Флешка",
  "i_led Name": "Транспортир LEDX", "i_led ShortName": "LEDX",
  "i_key Name": "Ключ от КПП", "i_key ShortName": "КПП",
  "MOD_STOCK": "Приклад"
}}
```

В `newServer` добавить `"/pve/items_ru": "testdata/pve_items_ru.json"` и отдавать 404 на этот путь при `fs.noItems`.

- [ ] **Step 2: Написать падающие тесты**

```go
func TestItemNames(t *testing.T) {
	s, _, _, _ := loaded(t)
	if n, ok := s.ItemName("i_led"); !ok || n != (ItemName{Name: "Транспортир LEDX", Short: "LEDX"}) {
		t.Errorf("i_led = %+v %v", n, ok)
	}
	if _, ok := s.ItemName("MOD_STOCK"); ok {
		t.Error("ключи без суффикса Name/ShortName — не предметы")
	}
	if _, ok := s.ItemName("nope"); ok {
		t.Error("нет такого предмета")
	}
}

func TestMapsWorkWithoutItemsFile(t *testing.T) {
	fs := newServer(t)
	fs.noItems = true
	c := &clock{t: time.Now()}
	dir := t.TempDir()
	s := newStore(t, fs, c, dir)
	if err := s.Load("pve", false); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.MapByScene("maps/shopping_mall.bundle"); !ok {
		t.Error("карта должна загрузиться без items_ru")
	}
	if _, ok := s.ItemName("i_led"); ok {
		t.Error("названий быть не должно")
	}
	if err := newStore(t, fs, c, dir).LoadCached("pve"); err != nil {
		t.Errorf("LoadCached без items_ru: %v", err)
	}
}

func TestStaleWantsItemsFileOnce(t *testing.T) {
	fs := newServer(t)
	fs.noItems = true
	c := &clock{t: time.Now()}
	dir := t.TempDir()
	s := newStore(t, fs, c, dir)
	for _, f := range s.files("pve") { // свежие обязательные файлы, items_ru нет
		if err := s.download(f.url, filepath.Join(dir, f.name)); err != nil {
			t.Fatal(err)
		}
	}
	if !s.Stale("pve") {
		t.Error("без items_ru данные считаются устаревшими — его надо попробовать скачать")
	}
	if err := s.Fetch("pve"); err != nil {
		t.Fatalf("Fetch не должен падать из-за items_ru: %v", err)
	}
	if s.Stale("pve") {
		t.Error("после неудачной попытки items_ru до конца запуска не устаревает")
	}
}
```

- [ ] **Step 3: Запустить — должен упасть**

Run: `... go test ./internal/data/ -run 'TestItemNames|TestMapsWorkWithoutItemsFile|TestStaleWantsItemsFileOnce'`
Expected: FAIL при компиляции: нет `ItemName`, `noItems`.

- [ ] **Step 4: Реализовать** по описанию выше.

- [ ] **Step 5: Запустить пакет — должно пройти** (в том числе тесты `Syncer` и проверки числа обращений к серверу)

Run: `... CGO_ENABLED=1 go test -race ./internal/data/`
Expected: PASS

- [ ] **Step 6: Коммит** — «Названия предметов из items_ru: необязательный файл, карта работает и без него».

---

### Task 5: Слой меток в core, вызовы для фронтенда, строка поиска в настройках

**Files:**
- Create: `internal/core/content.go`
- Modify: `internal/core/core.go` (`MapSource`, поле `gen`, `resolve`, `MapView.ContentGen`)
- Modify: `internal/core/core_test.go` (`fakeSrc` реализует новые методы; данные меток у Развязки)
- Modify: `internal/settings/settings.go` (`LootQuery string`), `internal/settings/settings_test.go`
- Modify: `app.go` (`GetMapContent`, `SetLootQuery`)
- Test: `internal/core/content_test.go`

**Interfaces:**
- Consumes: `data.Label`, `data.ItemName`, новые поля `data.GameMap` (Задачи 2–4), `mapgeo.LayerOfSpan` (Задача 1), `mapgeo.Project`, `mapgeo.LayerOf`.
- Produces:

```go
// MapSource дополняется:
//   Labels(normalizedName string) []data.Label
//   ItemName(id string) (data.ItemName, bool)

type Pt struct {
	PX, PY float64
	Layer  string
}
type PlacePt struct {
	Pt
	Text string
}
type ContainerPt struct {
	Pt
	Group, Name string
}
type LootPt struct {
	Pt
	Items []string
}
type LockPt struct {
	Pt
	Kind, Key  string // Key — название ключа; не найдено — ""
	NeedsPower bool
}
type NamedPt struct {
	Pt
	Name string
}
type HazardArea struct {
	Pt
	Kind    string
	Outline [][2]float64 // контур в координатах карты
}
type BossPt struct {
	Pt
	Name               string
	Chance, SpotChance float64
}
type MapContent struct {
	MapID        string
	Gen          int
	Places       []PlacePt
	Containers   []ContainerPt
	Loot         []LootPt
	Items        map[string]data.ItemName // только предметы, встречающиеся в Loot этой карты
	ItemsMissing bool                     // на карте есть лут, но ни одного названия
	Locks        []LockPt
	Switches     []NamedPt
	Hazards      []HazardArea
	Weapons      []NamedPt
	Bosses       []BossPt // по точке на каждую позицию
}
func (c *Core) Content() MapContent
// MapView получает поле ContentGen int (= c.gen).

// app.go
func (a *App) GetMapContent() core.MapContent
func (a *App) SetLootQuery(q string) error
// settings.Settings получает поле LootQuery string.
```

- `gen` растёт на каждом `resolve()` (смена карты и каждая `SetSource`, в том числе после фонового обновления данных).
- Все срезы — не `nil`, `Items` — не `nil`: фронтенд перебирает их без проверок (как в `View`).
- Этаж подписи: есть и `Bottom`, и `Top` — `LayerOfSpan`, иначе `def.SVGLayer`. Этаж остальных точек — `LayerOf` по позиции (у опасной зоны — по `Pos`).
- Карта не поддерживается — пустой `MapContent` с `MapID` и `Gen`.

- [ ] **Step 1: Написать падающие тесты** в `content_test.go`. `fakeSrc` для Развязки возвращает: 1 контейнер (`weapon`, «Оружейный кофр», `{X: 10, Y: 1, Z: 20}`), 1 точку лута на высоте 30 внутри ТЦ с предметами `i_led`, `i_unknown`, замок с `KeyID: "i_key"`, опасную зону с контуром из 3 точек, босса «Килла» с одной точкой (`Chance: 0.75`, `SpotChance: 0.33`); `Labels("interchange")` — «IDEA» (`Bottom 25`, `Top 33`, внутри ТЦ) и «Электростанция» (без высот); `ItemName` знает `i_led` и `i_key`.

```go
func TestContentProjectsAndResolves(t *testing.T) {
	c := New(fakeSrc{})
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvMap, Bundle: "maps/shopping_mall.bundle"})
	m := c.Content()
	px, py := mapgeo.Project(ic, 10, 20)
	if len(m.Containers) != 1 || m.Containers[0].PX != px || m.Containers[0].PY != py ||
		m.Containers[0].Group != "weapon" || m.Containers[0].Layer != "Ground_Level" {
		t.Errorf("Containers = %+v", m.Containers)
	}
	if len(m.Loot) != 1 || m.Loot[0].Layer != "First_Floor" {
		t.Errorf("Loot = %+v", m.Loot)
	}
	if len(m.Items) != 1 || m.Items["i_led"].Short != "LEDX" || m.ItemsMissing {
		t.Errorf("Items = %+v, missing=%v", m.Items, m.ItemsMissing)
	}
	if len(m.Locks) != 1 || m.Locks[0].Key != "Ключ от КПП" {
		t.Errorf("Locks = %+v", m.Locks)
	}
	if len(m.Hazards) != 1 || len(m.Hazards[0].Outline) != 3 {
		t.Errorf("Hazards = %+v", m.Hazards)
	}
	if len(m.Bosses) != 1 || m.Bosses[0].Name != "Килла" || m.Bosses[0].SpotChance != 0.33 {
		t.Errorf("Bosses = %+v", m.Bosses)
	}
	layer := map[string]string{}
	for _, p := range m.Places {
		layer[p.Text] = p.Layer
	}
	if layer["IDEA"] != "First_Floor" || layer["Электростанция"] != "Ground_Level" {
		t.Errorf("этажи подписей: %v", layer)
	}
}

func TestContentItemsMissing(t *testing.T) {
	c := New(noItemsSrc{}) // как fakeSrc, но ItemName всегда false
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvMap, Bundle: "maps/shopping_mall.bundle"})
	if m := c.Content(); !m.ItemsMissing || m.Items == nil {
		t.Errorf("ItemsMissing=%v Items=%v", m.ItemsMissing, m.Items)
	}
}

func TestContentEmptyForMapWithoutContent(t *testing.T) {
	c := New(fakeSrc{})
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvMap, Bundle: "maps/sandbox_start_preset.bundle"})
	b, _ := json.Marshal(c.Content())
	for _, k := range []string{"Places", "Containers", "Loot", "Locks", "Switches", "Hazards", "Weapons", "Bosses"} {
		if !strings.Contains(string(b), `"`+k+`":[]`) {
			t.Errorf("%s должен быть пустым списком: %s", k, b)
		}
	}
	if !strings.Contains(string(b), `"Items":{}`) || strings.Contains(string(b), `"ItemsMissing":true`) {
		t.Errorf("Items/ItemsMissing: %s", b)
	}
}

func TestContentGenGrowsOnSetSource(t *testing.T) {
	c := New(fakeSrc{})
	c.OnLogEvent(gamelog.Event{Kind: gamelog.EvMap, Bundle: "maps/shopping_mall.bundle"})
	g := c.View().ContentGen
	c.SetSource(fakeSrc{}) // фоновое обновление данных
	if v := c.View(); v.ContentGen <= g || c.Content().Gen != v.ContentGen {
		t.Errorf("ContentGen %d → %d, Content().Gen %d", g, v.ContentGen, c.Content().Gen)
	}
}
```

В `internal/settings/settings_test.go`:

```go
func TestLootQueryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := Load(dir)
	s.LootQuery = "флешка"
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}
	if got := Load(dir).LootQuery; got != "флешка" {
		t.Errorf("LootQuery = %q", got)
	}
}
```

- [ ] **Step 2: Запустить — должны упасть**

Run: `... go test ./internal/core/ ./internal/settings/`
Expected: FAIL при компиляции: нет `Content`, `ContentGen`, `LootQuery`, у `fakeSrc` нет новых методов интерфейса.

- [ ] **Step 3: Реализовать** `content.go`, изменения `core.go`, поле настроек и два метода `App`.

- [ ] **Step 4: Запустить всё — должно пройти**

Run: `MSYS_NO_PATHCONV=1 wsl -e bash -lc 'cd /mnt/c/Users/vadimushka_d/Projects/tarkov-map && CGO_ENABLED=1 go test -race ./... && go vet ./...'`
Expected: PASS, `go vet` без замечаний

- [ ] **Step 5: Коммит** — «Слой меток для фронтенда: GetMapContent, ContentGen, строка поиска в настройках».

---

### Task 6: Проверка на реальных данных и данные для браузерной проверки

**Files:**
- Modify: `internal/data/real_test.go`
- Create: `internal/core/real_test.go`

**Interfaces:**
- Consumes: всё из Задач 2–5.
- Produces: тест `TestDumpRealContent` (пропускается без `TM_REAL_DATA` и `TM_DUMP`): копирует `maps.json`, `pve_maps.json`, `pve_maps_ru.json` и, если есть, `pve_items_ru.json` из `TM_REAL_DATA` во временную папку, делает `Store.Load("pve", false)` (докачает `items_ru` и SVG по сети), и для карт `interchange` (`maps/shopping_mall.bundle`), `streets-of-tarkov` (`maps/city_preset.bundle`), `shoreline` (`maps/shoreline_preset.bundle`) пишет в `TM_DUMP/<normalizedName>/` файлы `view.json` (`View()`), `content.json` (`Content()`), `map.svg` (`Store.SVG`). Кэш пользователя не изменяется.

- [ ] **Step 1: Дополнить `TestParseRealFiles`** проверками, которые падают без новых данных:

```go
	if len(ic.Containers) != 823 || len(ic.Loose) != 492 || len(ic.Locks) != 15 || len(ic.Hazards) != 25 ||
		len(ic.Switches) != 6 || len(ic.Weapons) != 2 || len(ic.Bosses) != 2 {
		t.Errorf("метки Развязки: контейнеров %d, лута %d, замков %d, зон %d, рубильников %d, пулемётов %d, боссов %d",
			len(ic.Containers), len(ic.Loose), len(ic.Locks), len(ic.Hazards), len(ic.Switches), len(ic.Weapons), len(ic.Bosses))
	}
	floors := map[string]int{}
	for _, l := range labels["interchange"] {
		if l.Bottom != nil && l.Top != nil {
			floors[mapgeo.LayerOfSpan(defs["interchange"], l.X, l.Z, *l.Bottom, *l.Top)]++
		} else {
			floors[defs["interchange"].SVGLayer]++
		}
	}
	if floors["Ground_Level"] != 14 || floors["First_Floor"] != 33 || floors["Second_Floor"] != 30 {
		t.Errorf("этажи подписей Развязки: %v", floors)
	}
	for _, l := range labels["shoreline"] {
		if l.Bottom != nil && l.Top != nil && mapgeo.LayerOfSpan(defs["shoreline"], l.X, l.Z, *l.Bottom, *l.Top) != defs["shoreline"].SVGLayer {
			t.Errorf("Берег: подпись %q не на земле", l.Text)
		}
	}
```

(числа — из кэша PvE от 29.10; если данные tarkov.dev с тех пор изменились, сверить с кэшем и поправить числа, а не код)

- [ ] **Step 2: Написать `TestDumpRealContent`** в `internal/core/real_test.go`.

- [ ] **Step 3: Запустить оба на кэше**

Run: `MSYS_NO_PATHCONV=1 wsl -e bash -lc 'cd /mnt/c/Users/vadimushka_d/Projects/tarkov-map && TM_REAL_DATA=/mnt/c/Users/vadimushka_d/AppData/Local/TarkovMap TM_DUMP=<scratchpad>/front/real go test ./internal/data/ ./internal/core/ -run "TestParseRealFiles|TestDumpRealContent" -v'` (`<scratchpad>` — WSL-путь к папке scratchpad сессии)
Expected: PASS; в `front/real/` три папки по три файла; `content.json` Улиц — 1282 контейнера и `"ItemsMissing":false`

- [ ] **Step 4: Коммит** — «Проверка меток на реальных данных и выгрузка для браузерной проверки».

---

### Task 7: Вкладка «Лут и места» и слои на карте

**Files:**
- Modify: `frontend/dist/index.html` (вкладка и панель)
- Modify: `frontend/dist/style.css` (цвета, `.n`, `.empty`, `.place`, `#tip`)
- Modify: `frontend/dist/map.js` (слои меток)
- Modify: `frontend/dist/app.js` (флажки, загрузка `GetMapContent`, числа)

**Interfaces:**
- Consumes: `GetMapContent()`, `MapView.ContentGen` (Задача 5).
- Produces (`window.TMap`): `setContent(content)`; `setFilter(key, on)` понимает новые ключи; `setLootQuery(q) → number` (Задача 8 наполняет; здесь — заглушка, возвращающая 0).

**Ключи флажков** (`data-f`) и значения по умолчанию, дописываются в `FILTER_DEFAULTS`:
`places: true`; `c_weapon`, `c_med`, `c_tech`, `c_safe`, `c_cache`, `c_body`, `c_misc`, `loot_all`, `locks`, `switches`, `hazards`, `weapons`, `bosses` — `false`.

**Разметка.** Вкладка `<button class="tab" role="tab" data-tab="loot">Лут и места</button>` между «Главная» и «Настройки»; панель `data-pane="loot"` с группами (`section.group`, подпись `.caption`):
- «Места»: `places` — «Названия мест» (значок `.sw.txt` «Аа»).
- «Контейнеры» (`.items.grid`): Оружейные, Медицина, Техника, Сейфы и кассы, Тайники, Тела, Сумки и прочее — у каждого цветной `.sw` и `<span class="n" data-n="c_…">`.
- «Лут на земле» (`.items.col`): строка с `<input id="loot-q" type="search" placeholder="Флешка, Ледокол…" title="Ищет только лут на земле; содержимое контейнеров неизвестно" spellcheck="false">` и кнопкой `<button id="loot-clear" title="Очистить">×</button>`; `<span id="loot-found" class="muted">`; флажок `loot_all` «Все точки» с `data-n="loot_all"`.
- «Прочее» (`.items.grid`): Замки, Рубильники, Опасные зоны, Пулемёты, Боссы — с `data-n`.

**Цвета** (`:root`): `--c-weapon: #d98c5f; --c-med: #ef6f9a; --c-tech: #8fa8ff; --c-safe: #f5f0e1; --c-cache: #a3b86c; --c-body: #b0a8a0; --c-misc: #8c7b65; --loot: #9a9488; --loot-hit: #ffe14d; --lock: #e0c27a; --switch: #ffd23f; --mg: #ff8a3d; --boss: #ff3b3b; --hazard: #ff6b4a; --sniper: #ffaa00; --place: #cfc8b4`.

**Значки** (в пикселях экрана через существующий механизм `pin`):

| Слой | Вид | Подсказка |
|---|---|---|
| places | `text.place`: 11.5px, вес 500, `fill: var(--place)`, тёмная обводка `paint-order: stroke`, `pointer-events: none`, по центру точки | — |
| c_* | круг r 3.5, заливка цветом группы, обводка `rgba(0,0,0,.8)` 1 | `Name` |
| loot_all | круг r 2.5, `--loot` | см. Задачу 8 |
| locks | квадрат 8×8, rx 1.5, `--lock`, обводка чёрная | «Дверь» / «Багажник» / «Замок» · «ключ: <Key>» (пустой — «ключ неизвестен») · «нужно питание» |
| switches | круг r 4.5, `--switch`, обводка чёрная | `Name` |
| weapons | треугольник `M0,-6 L5.5,4 L-5.5,4 Z`, `--mg` | `Name` |
| bosses | ромб `M0,-7 L7,0 L0,7 L-7,0 Z`, `--boss`, белая обводка 1.5 | «<Name> · появляется в 75% рейдов · это место — 33%» (проценты округлены) |
| hazards | `polygon` в координатах карты по `Outline`: заливка цветом зоны с прозрачностью .15, пунктир `5 4`, толщина 1.5, `vector-effect: non-scaling-stroke`; меньше 3 точек — круг r 6 в точке | `minefield` — «Минное поле», `sniper` — «Зона снайпера», прочее — «Опасная зона» |

Цвет зоны: `sniper` — `--sniper`, остальные — `--hazard`.

**Порядок слоёв** (снизу вверх), все — в отдельной группе под существующими группами выходов и маршрута: hazards, places, c_misc, c_body, c_cache, c_tech, c_med, c_safe, c_weapon, loot_all, locks, switches, weapons, bosses, loot_hit.

**Поведение map.js:**
- Слой строится (DOM) только когда включён: при `setContent` — включённые; выключенный — при первом включении. `setContent` сбрасывает все построенные слои меток.
- `drawMarkers` (выходы, маршрут) не трогает слои меток и их метки; массив `pins` остаётся только для выходов и маршрута, у каждого слоя меток — свой список.
- `draw()` пересчитывает положение меток выходов и маршрута и только **видимых** слоёв меток; слой, показанный после изменения масштаба, сначала пересчитывается.
- Подсказки меток — одна пара обработчиков (`mousemove`/`mouseleave`) на группу слоёв меток; текст строится при наведении.
- Этажи: значки и контуры подчиняются существующему правилу (`data-layer`, чужой этаж — прозрачность .35). Подписи мест не приглушаются; подпись не базового этажа видна только при `floor === Layer`, подписи базового — всегда.
- `clear()` и `setSVG()` сбрасывают и слои меток.

**Поведение app.js:**
- В `updateView`, после возможного `setSVG` и до `render`: если `v.MapID + "#" + v.ContentGen` отличается от загруженного, вызвать `GetMapContent()`. Ответ с другими `MapID`/`Gen` — не применять и повторить цикл (`again = true`). Иначе: `TMap.setContent(c)`, заполнить числа, применить текущую строку поиска (`TMap.setLootQuery`).
- Числа (`[data-n]`): группы контейнеров — по `Group`; `loot_all` — `Loot.length`; `locks`, `switches`, `hazards`, `weapons` — длины списков; `bosses` — число разных `Name`. Ноль — у `label` класс `empty` (`opacity: .45`), флажок остаётся рабочим.
- Неподдерживаемая карта — числа пустые, загруженный ключ сбрасывается.

- [ ] **Step 1: Подготовить браузерную проверку.** В `<scratchpad>/front/` скопировать текущие `frontend/dist/*`; в `index.html` перед `map.js` подключить `stub.js` и поднять `?v=N` у скриптов. `stub.js`: параметр `?map=<normalizedName>` берёт `view.json`, `content.json`, `map.svg` из `real/<map>/`; `GetMapContent` возвращает `content.json` с `Gen` и `MapID` как у вида; `GetSettings` возвращает `{Side, Filters, LootQuery}`; `SetLootQuery` запоминает строку; `window.__bumpGen()` увеличивает `ContentGen`/`Gen` и вызывает обработчик события `view`. В `C:\Users\vadimushka_d\.claude\launch.json` — конфигурация `tarkov-front-check`: `wsl -e python3 -m http.server 8765 --directory <WSL-путь к front>`.

- [ ] **Step 2: Реализовать разметку, стили, map.js и app.js** по описанию выше.

- [ ] **Step 3: Проверить в браузерной панели** (`preview_start` с `tarkov-front-check`, `?map=interchange`), каждое — с ожидаемым результатом:
  - вкладка «Лут и места» открывается; у Развязки числа: Оружейные 178, Медицина 21, Техника 124, Сейфы и кассы 130, Тайники 89, Тела 6, Сумки и прочее 275, «Все точки» 492, Замки 15, Рубильники 6, Опасные зоны 25, Пулемёты 2, Боссы 2;
  - по умолчанию видны только названия мест; в DOM нет построенных слоёв контейнеров;
  - включение «Оружейные» показывает значки, подсказка при наведении — «Оружейный кофр» и т.п.;
  - выбор 2-го этажа показывает подписи магазинов 2-го этажа и скрывает 3-го; на основном этаже подписей магазинов нет, «Электростанция» видна на всех этажах; значки чужого этажа приглушены;
  - `window.__ev.view()` с добавленным скрином и смена стороны ЧВК/Дикий не меняют число детей у построенных слоёв меток и не скрывают их;
  - `window.__bumpGen()` пересобирает слои; флажки остаются как были;
  - заглушка с пустыми списками в `content.json` (скопировать и обнулить) — в ленте нули с классом `empty`, ошибок нет;
  - консоль — без ошибок (`read_console_messages`, `onlyErrors`).

- [ ] **Step 4: Проверить плавность на Улицах** (`?map=streets-of-tarkov`, включить все слои): через `javascript_tool` замерить 20 шагов колеса (`dispatchEvent(new WheelEvent(...))`) с `performance.now()` до и после.
Expected: в среднем не больше 8 мс на шаг; иначе — найти узкое место (профиль `draw`/`applyFloor`) и исправить до перехода дальше.

- [ ] **Step 5: Коммит** — «Вкладка «Лут и места»: названия мест, контейнеры и прочие метки на карте».

---

### Task 8: Поиск лута на земле

**Files:**
- Modify: `frontend/dist/map.js` (`setLootQuery`, слои `loot_all` и `loot_hit`, подсказка точки лута)
- Modify: `frontend/dist/app.js` (поле поиска, сохранение строки, «найдено N»)
- Modify: `frontend/dist/style.css` (ширина `#loot-q`)

**Interfaces:**
- Consumes: `MapContent.Loot`, `Items`, `ItemsMissing`; `SetLootQuery`, `Settings.LootQuery` (Задача 5).
- Produces: `TMap.setLootQuery(q) → number` — число точек, где есть совпавший предмет (0, если после `trim` меньше 2 символов).

**Правила:**
- Нормализация строки и названий: `toLowerCase()`, `ё` → `е`. Совпадение — подстрока в `Name` или `Short`.
- `loot_hit` (совпавшие точки: круг r 5, `--loot-hit`, обводка чёрная 1.2) виден всегда, когда есть строка поиска, независимо от флажка `loot_all`. `loot_all` показывает остальные точки серыми.
- Подсказка точки лута: первая строка «Может появиться:», дальше названия (`Name`), сначала совпавшие, не больше 8; остальное — «ещё N». Нет названий (`ItemsMissing`) — «Названия предметов не загружены». Строки — через перевод строки, у `#tip` `white-space: pre-line`.
- app.js: ввод — задержка 150 мс, затем `TMap.setLootQuery(q)` и `SetLootQuery(q)`; `#loot-found`: пусто при строке короче 2 символов, «найдено N точек» с правильным окончанием (точка / точки / точек), «не найдено» при нуле; `×` очищает поле и применяет сразу. При запуске поле заполняется из `Settings.LootQuery`; при новой карте строка применяется заново. `ItemsMissing` — поле неактивно, `placeholder` «Нет названий предметов — нажмите «Обновить данные»».

- [ ] **Step 1: Реализовать** по правилам выше.

- [ ] **Step 2: Проверить в браузерной панели** (`?map=interchange`), каждое — с ожидаемым результатом:
  - «LEDX» — подсвечены точки, «найдено N точек» совпадает с числом жёлтых значков; подсказка начинается с «Может появиться:», LEDX — первым;
  - «флешка» и «ФЛЕШКА» дают одно и то же; строка с `ё` находит название с `е` и наоборот;
  - «л» (1 символ) — подсветки нет, `#loot-found` пуст;
  - «zzzz» — «не найдено», подсвеченных точек нет;
  - `×` очищает поле, подсветка исчезает, `SetLootQuery("")` вызван;
  - с «Все точки» и строкой поиска видны и серые, и жёлтые точки, жёлтые сверху;
  - перезагрузка страницы с `LootQuery` в заглушке — строка в поле и подсветка восстановлены;
  - `ItemsMissing: true` в заглушке — поле неактивно с подсказкой про «Обновить данные»;
  - консоль без ошибок.

- [ ] **Step 3: Коммит** — «Поиск лута на земле по названию предмета».

---

### Task 9: Иконка приложения

Владелец попросил иконку (30.10). Сейчас у exe стандартный значок Windows.

**Files:**
- Create: `assets/appicon.png` (256×256), генератор `scripts/icon/main.go` (чистый Go, стандартная библиотека)
- Create: ресурс Windows `rsrc_windows_amd64.syso` в корне модуля (подхватывается `go build` для `GOOS=windows` автоматически)
- Modify: `scripts/build.sh` — только если ресурс решено генерировать при сборке, а не хранить в репозитории

**Решения:**
- Рисунок в стиле приложения: тёмный скруглённый квадрат (`#14130f`), сложенная карта песочного цвета (`#e0c27a`, сгибы темнее) и красная метка «я здесь» с конусом взгляда (`#ff5d5d`, белая обводка) — как на карте. Сглаживание — суперсэмплинг 4×4.
- Ресурс — через `github.com/tc-hib/go-winres` (`go run …@<версия> simply --icon assets/appicon.png`), иконки 16–256 px. Манифест не добавлять, если без него exe работает как сейчас (манифест меняет поведение DPI); если `simply` требует манифест — проверить, что масштаб окна не изменился.
- Иконка окна и панели задач: проверить в исходниках Wails v2 (`internal/frontend/desktop/windows`), под каким id он ищет иконку в ресурсах, и задать этот id.

- [ ] **Step 1: Нарисовать** — написать генератор, получить `assets/appicon.png`, посмотреть картинку (Read) и уменьшенную до 32 px: метка и карта различимы.
- [ ] **Step 2: Сделать ресурс** `.syso` и проверить сборку: `./scripts/build.sh` без ошибок.
- [ ] **Step 3: Проверить** в Windows: у `build\bin\TarkovMap.exe` в проводнике новая иконка (`[System.Drawing.Icon]::ExtractAssociatedIcon` в PNG и посмотреть), окно приложения и панель задач — тоже (снимок окна). Ярлык на рабочем столе показывает новую иконку (при необходимости обновить кэш значков `ie4uinit.exe -show`).
- [ ] **Step 4: Коммит** — «Иконка приложения».

---

### Task 10: Сборка и проверка в настоящем приложении

**Files:** без изменений кода (только если проверка найдёт ошибку — тогда тест/исправление и отдельный коммит).

- [ ] **Step 1: Прогнать все тесты**

Run: `MSYS_NO_PATHCONV=1 wsl -e bash -lc 'cd /mnt/c/Users/vadimushka_d/Projects/tarkov-map && CGO_ENABLED=1 go test -race ./... && go vet ./...'`
Expected: PASS

- [ ] **Step 2: Собрать exe.** Если `TarkovMap.exe` запущен, а игра (`EscapeFromTarkov`) — нет, закрыть приложение через `CloseMainWindow`; если игра запущена — не трогать, собрать под именем `build/bin/TarkovMapCheck.exe`.

Run: `MSYS_NO_PATHCONV=1 wsl -e bash -lc 'cd /mnt/c/Users/vadimushka_d/Projects/tarkov-map && ./scripts/build.sh'`
Expected: `build/bin/TarkovMap.exe` обновлён, без ошибок

- [ ] **Step 3: Запустить и проверить** (только если игра не запущена): приложение стартует; в `%LOCALAPPDATA%\TarkovMap` появился `pve_items_ru.json`; снимок окна через PrintWindow (`pwshot.ps1` из scratchpad) с открытой вкладкой «Лут и места» показывает ленту с группами и названия мест на карте.
Expected: окно открывается, на последней карте видны названия мест, вкладка с числами

- [ ] **Step 4: Записать итог** владельцу: что проверено в браузере и в приложении, что осталось проверить в рейде (подписи на своих местах, поиск находит реальный лут).
