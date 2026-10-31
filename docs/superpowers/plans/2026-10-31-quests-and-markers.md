# Квесты, спавны и карточки — план

> Для исполнителя: superpowers:executing-plans, задачи по порядку, TDD для Go.

**Цель:** взять у tarkov.dev, tarkov-market и TarkovPilot то, что подходит офлайн-приложению: новые метки из
снимка, карточку по клику, часы и таймер рейда, квесты с активными заданиями из логов, расстояние до выходов.

**Архитектура:** `internal/data` разбирает новые поля снимка, `internal/core` проецирует их в координаты карты
(`MapContent`, `MapView`), фронтенд (`frontend/dist`, без сборщика) рисует. Задания — новые файлы снимка
`{mode}_tasks.json` и `{mode}_tasks_ru.json`, состояние заданий — из `push-notifications_*.log`.

**Решения пользователя (31.10.2026):** спутниковую подложку не берём; остальной план (этапы 1, 2, 4) согласован.

## Общие ограничения
- В сеть приложение не ходит; данные качает только `scripts/seed`.
- Не трогать процесс и память игры, скриншоты не открывать и не удалять.
- Коммиты на русском, с трейлером `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`; почта `vadimushka.d@gmail.com`.
- Тесты: `CGO_ENABLED=1 go test -race ./...` в WSL; gofmt чистый.
- Фронтенд — чистый JS, метки в пикселях экрана (`pin`/`cpin`), этажи через `data-layer`.

## Факты о данных (проверено на снимке и логах)
- `maps[].spawns[]`: `position`, `sides` (`pmc`|`scav`|`all`), `categories` (`player`, `bot`, `boss`, `botpmc`),
  `zoneName` (у снайперов содержит `sniper`, без регистра).
- `maps[].extracts[]`: `outline[]`, `top`, `bottom`, `switches[]` (id рубильников), `transferItem {item, count}`.
- `maps[].switches[]`: `id`, `name` (ключ перевода), `activates[] {operation, switch}`, `activatedBy`.
- `maps[].btrStops[]`: `name` (ключ перевода), `x`, `y`, `z`.
- `maps[].raidDuration` (минуты), `players` ("11-15").
- `json.tarkov.dev/{mode}/tasks`: `data.tasks` (словарь или список); задание: `id`, `name` (ключ перевода),
  `trader.name`, `map`, `kappaRequired`, `objectives[] {id, description, type, optional, zones[] {map, position,
  outline[], top, bottom}, possibleLocations[] {map, positions[]}, questItem {name}, requiredKeys}`.
  `{mode}/tasks_ru`: `data` — словарь переводов.
- `push-notifications_*.log`: строка `Got notification | ChatMessageReceived`, следом JSON до строки с датой;
  `message.type` 10 — задание начато, 11 — провалено, 12 — выполнено; `message.templateId` = «<questId> …».
  Режим уведомления — последняя перед ним строка `Session mode: Pve|Regular` в application-логе той же сессии.
- Строка `[Transit] … Locations:Interchange -> ` пишется перед каждым рейдом с пустым направлением — признаком
  транзита не служит; настоящего транзита в логах пока нет (пункт про транзит отложен).

## Задачи

### 1. Новые метки из снимка (data → core)
- `data.GameMap`: `Spawns []Spawn{Kind: "pmc"|"scav"|"sniper", Pos}` (боссовые спавны не дублируем — боссы уже
  есть), `BTRStops []NamedPos`, `RaidMinutes int`; `Extract` + `Outline []Vec3`, `Item string` (id),
  `ItemCount int`, `Switches []string`; `Switch` + `ID`, `Activates []string` (id).
- `core.MapContent`: `Spawns []SpawnPt`, `BTR []NamedPt`; `Switches` с `ID`, `Activates []string`.
  `core.Marker` (выход): `Outline [][2]float64`, `Need string` («Рубли ×20 000»), `SwitchNames []string`.
  `core.MapView.RaidMinutes`.
- Тесты: разбор на маленьком снимке (`snapshot(t)` в `store_test.go`), проекция в `content_test.go`.

### 2. Новые слои и карточка (фронтенд)
- Вкладка «Лут и места»: группа «Спавны» — ЧВК, Дикие, Снайперы; в «Прочее» — «Остановки БТР».
- Контур зоны выхода и транзита — при наведении на значок.
- Карточка по клику на любую метку (`#card`): заголовок, строки, ссылки «показать» для связанных объектов
  (рубильник → что открывает; выход → нужные рубильники), клик по ссылке центрирует карту и включает этаж.
- Группировка: в слоях контейнеров, лута и квестов метки ближе 22 px на экране сливаются в кружок с числом;
  клик по кружку приближает. Пересборка при смене масштаба.
- Ctrl + колесо — этаж выше/ниже.

### 3. Часы и таймер рейда
- Статусная строка: игровое время (два значения; Factory — фиксированное 15:28/03:28) и «до конца рейда мм:сс»
  от `RaidStart` + `RaidMinutes`. Формула часов: `(UTC мс × 7 + 3 ч [+12 ч]) mod 24 ч`.

### 4. Поиск через запятую
- Строка поиска «Лут на земле» делится по запятым; совпадение по любому слову. Дополнительно подсвечиваются
  контейнеры по названию и замки по названию ключа.

### 5. Задания в снимке (seed + data)
- `scripts/seed`: качает `{mode}/tasks` и `{mode}/tasks_ru`, проверяет, что заданий > 0.
- `data`: `Tasks(mapID) []Task` — задания, у целей которых есть места на этой карте: `ID, Name, Trader, Kappa`,
  `Objectives []Objective{ID, Text, Optional, Zones [][]Vec3 (контуры), Points []Vec3, Item string, Keys []string}`.
  Файлы необязательны: нет — квестов нет, остальное работает.

### 6. Состояние заданий из логов (gamelog)
- `gamelog.QuestEvent{At, Mode, QuestID, Status: start|fail|done}`; разбор блока push-notifications.
- `gamelog.ScanQuests(logsDir) map[mode]map[questID]status` — все сессии по порядку, последнее событие побеждает.
- Хвост: `Tailer` читает и `push-notifications_*.log`, события идут в `App`.

### 7. Квесты в интерфейсе
- Вкладка «Задания»: список заданий карты (галочка, торговец, «Каппа»), кнопки «Все»/«Ничего», фильтр
  «Только активные» (по логам), поиск по названию. Выбор хранится в настройках (`HiddenTasks`).
- Слой на карте: зоны целей — контур, места предметов — значок; карточка: задание, цель, ключи.

### 8. Расстояние до выходов
- `core.Marker.Dist` — метры по горизонтали от последнего скрина рейда до выхода; в подсказке и карточке;
  в статусе — ближайший свой выход.

### 9. Проверка и сборка
- `go test -race ./...`, Windows-тесты, проверка фронтенда в браузере на заглушке, сборка exe, ревью ветки.
