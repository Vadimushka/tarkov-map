// Package data читает карты и метки tarkov.dev из снимка, встроенного в exe
// (его собирает scripts/seed). В сеть приложение не ходит: новые данные — новая сборка.
package data

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"sync"
	"time"

	"github.com/Vadimushka/tarkov-map/internal/mapgeo"
)

var ErrNoData = errors.New("в программе нет данных карт")

// Extract — выход с карты. Faction: "pmc" | "scav" | "shared". Item и ItemCount — плата за
// выход (id предмета, обычно рубли), Switches — id рубильников, которые его открывают.
type Extract struct {
	Name, Faction string
	Pos           mapgeo.Vec3
	Paid, Switch  bool
	Outline       []mapgeo.Vec3
	Item          string
	ItemCount     int
	Switches      []string
}

type Transit struct {
	Name    string
	Pos     mapgeo.Vec3
	Outline []mapgeo.Vec3
}

type GameMap struct {
	ID, NormalizedName, Name, ScenePath string
	Extracts                            []Extract
	Transits                            []Transit
	Containers                          []Container
	Loose                               []Loose
	Locks                               []Lock
	Switches                            []Switch
	Hazards                             []Hazard
	Weapons                             []Weapon
	Bosses                              []Boss
	Spawns                              []Spawn
	BTRStops                            []NamedPos
	RaidMinutes                         int // длительность рейда; 0 — неизвестна
}

// Варианты карт, у которых своей SVG нет.
var aliases = map[string]string{
	"night-factory":        "factory",
	"ground-zero-21":       "ground-zero",
	"ground-zero-tutorial": "ground-zero",
}

// Store — данные одного режима из снимка. Файлы снимка: maps.json, {mode}_maps.json,
// {mode}_maps_ru.json, {mode}_items_ru.json (необязателен: без него нет только поиска
// лута), {mode}_tasks.json и {mode}_tasks_ru.json (необязательны: без них нет квестов)
// и svg/<имя>.svg.
type Store struct {
	src fs.FS

	mu      sync.RWMutex
	mode    string
	byScene map[string]GameMap
	defs    map[string]mapgeo.MapDef
	labels  map[string][]Label
	items   map[string]ItemName
	updated time.Time

	tasks     map[string][]Task // по id карты
	taskCount int
}

// NewStore читает снимок src; nil — снимка нет, Load вернёт ErrNoData.
func NewStore(src fs.FS) *Store {
	return &Store{src: src}
}

// Load делает текущими данные режима ("pve" | "regular"). Тот же режим повторно не
// разбирается: Load зовётся на каждую загрузку карты игрой.
func (s *Store) Load(mode string) error {
	s.mu.RLock()
	same := s.mode == mode
	s.mu.RUnlock()
	if same {
		return nil
	}
	if s.src == nil {
		return ErrNoData
	}
	defs, labels, err := parseDefs(s.src, "maps.json")
	if err != nil {
		return noData("maps.json", err)
	}
	maps, err := parseMaps(s.src, mode+"_maps.json", mode+"_maps_ru.json")
	if err != nil {
		return noData(mode+"_maps.json", err)
	}
	items := parseItems(s.src, mode+"_items_ru.json")
	tasks, taskCount := parseTasks(s.src, mode+"_tasks.json", mode+"_tasks_ru.json", items)
	var updated time.Time
	if st, err := fs.Stat(s.src, mode+"_maps.json"); err == nil {
		updated = st.ModTime()
	}
	s.mu.Lock()
	s.mode, s.defs, s.labels, s.items, s.byScene, s.updated = mode, defs, labels, items, maps, updated
	s.tasks, s.taskCount = tasks, taskCount
	s.mu.Unlock()
	return nil
}

// noData: файла нет в снимке — ErrNoData, иначе файл не разбирается.
func noData(name string, err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w (%s)", ErrNoData, name)
	}
	return fmt.Errorf("%s: %w", name, err)
}

func (s *Store) MapByScene(scenePath string) (GameMap, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.byScene[scenePath]
	return m, ok
}

// MapRef — карта в списке ручного выбора.
type MapRef struct{ Scene, ID, Name string }

// Maps — карты, которые можно показать (есть SVG), по названию.
func (s *Store) Maps() []MapRef {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []MapRef{}
	for scene, m := range s.byScene {
		id := m.NormalizedName
		if a, ok := aliases[id]; ok {
			id = a
		}
		if _, ok := s.defs[id]; ok {
			out = append(out, MapRef{Scene: scene, ID: m.NormalizedName, Name: m.Name})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ItemName — название предмета по id; false — нет такого или нет файла названий.
func (s *Store) ItemName(id string) (ItemName, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.items[id]
	return n, ok
}

// ItemCount — сколько предметов с названиями в данных режима (0 — нет файла названий).
func (s *Store) ItemCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}

// MapDef возвращает параметры SVG-карты; false — карта без SVG.
func (s *Store) MapDef(normalizedName string) (mapgeo.MapDef, bool) {
	if a, ok := aliases[normalizedName]; ok {
		normalizedName = a
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.defs[normalizedName]
	return d, ok
}

// SVGDefs — все SVG-карты из maps.json, каждая один раз (для снимка данных).
func (s *Store) SVGDefs() []mapgeo.MapDef {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen, out := map[string]bool{}, []mapgeo.MapDef{}
	for _, d := range s.defs {
		if !seen[d.SVGPath] {
			seen[d.SVGPath] = true
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SVGPath < out[j].SVGPath })
	return out
}

// Labels — подписи мест SVG-карты (варианты карт без своей SVG — по aliases).
func (s *Store) Labels(normalizedName string) []Label {
	if a, ok := aliases[normalizedName]; ok {
		normalizedName = a
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.labels[normalizedName]
}

// Updated — время снимка (когда scripts/seed скачал данные режима).
func (s *Store) Updated() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.updated
}

// SVG — текст SVG-карты из снимка.
func (s *Store) SVG(def mapgeo.MapDef) (string, error) {
	if s.src == nil {
		return "", ErrNoData
	}
	b, err := fs.ReadFile(s.src, SVGName(def))
	return string(b), err
}

// SVGName — путь SVG-карты в снимке.
func SVGName(def mapgeo.MapDef) string {
	return "svg/" + path.Base(def.SVGPath)
}

// --- разбор maps.json (tarkov-dev) ---

type rawDef struct {
	Key                string         `json:"key"`
	Projection         string         `json:"projection"`
	Transform          []float64      `json:"transform"`
	CoordinateRotation float64        `json:"coordinateRotation"`
	Bounds             [2][2]float64  `json:"bounds"`
	SVGBounds          *[2][2]float64 `json:"svgBounds"`
	SVGPath            string         `json:"svgPath"`
	SVGLayer           string         `json:"svgLayer"`
	Labels             []rawLabel     `json:"labels"`
	Layers             []struct {
		Name     string `json:"name"`
		SVGLayer string `json:"svgLayer"`
		Extents  []struct {
			Height [2]float64        `json:"height"`
			Bounds []json.RawMessage `json:"bounds"` // [[x,z],[x,z],"имя"]
		} `json:"extents"`
	} `json:"layers"`
}

func parseDefs(src fs.FS, name string) (map[string]mapgeo.MapDef, map[string][]Label, error) {
	b, err := fs.ReadFile(src, name)
	if err != nil {
		return nil, nil, err
	}
	var raw []struct {
		NormalizedName string   `json:"normalizedName"`
		Maps           []rawDef `json:"maps"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, nil, err
	}
	out, labels := map[string]mapgeo.MapDef{}, map[string][]Label{}
	for _, m := range raw {
		for _, d := range m.Maps {
			if d.Projection != "interactive" || d.SVGPath == "" {
				continue
			}
			out[m.NormalizedName] = toDef(d)
			labels[m.NormalizedName] = toLabels(d.Labels)
			break
		}
	}
	return out, labels, nil
}

func toDef(d rawDef) mapgeo.MapDef {
	md := mapgeo.MapDef{
		Key: d.Key, Transform: d.Transform, CoordinateRotation: d.CoordinateRotation,
		Bounds: d.Bounds, SVGBounds: d.SVGBounds, SVGPath: d.SVGPath, SVGLayer: d.SVGLayer,
	}
	for _, l := range d.Layers {
		layer := mapgeo.Layer{Name: l.Name, SVGLayer: l.SVGLayer}
		if layer.SVGLayer == "" {
			// этаж есть только тайлами: id, которого нет в SVG, — фронтенд покажет базовый слой
			layer.SVGLayer = "tile:" + l.Name
		}
		for _, e := range l.Extents {
			ext := mapgeo.Extent{Height: e.Height}
			for _, rb := range e.Bounds {
				var parts []json.RawMessage
				if json.Unmarshal(rb, &parts) != nil || len(parts) < 2 {
					continue
				}
				var a, c [2]float64
				if json.Unmarshal(parts[0], &a) != nil || json.Unmarshal(parts[1], &c) != nil {
					continue
				}
				ext.Bounds = append(ext.Bounds, [2][2]float64{a, c})
			}
			layer.Extents = append(layer.Extents, ext)
		}
		md.Layers = append(md.Layers, layer)
	}
	return md
}

// --- разбор json.tarkov.dev/{mode}/maps ---

type rawMap struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	NormalizedName string `json:"normalizedName"`
	ScenePath      string `json:"scenePath"`
	RaidDuration   int    `json:"raidDuration"`
	Extracts       []struct {
		Name         string          `json:"name"`
		Faction      string          `json:"faction"`
		Switch       json.RawMessage `json:"switch"` // bool или id рубильника
		Switches     []string        `json:"switches"`
		Position     mapgeo.Vec3     `json:"position"`
		Outline      []mapgeo.Vec3   `json:"outline"`
		TransferItem *struct {
			Item  string `json:"item"`
			Count int    `json:"count"`
		} `json:"transferItem"`
	} `json:"extracts"`
	Transits []struct {
		Description string        `json:"description"`
		Position    mapgeo.Vec3   `json:"position"`
		Outline     []mapgeo.Vec3 `json:"outline"`
	} `json:"transits"`
	rawContent
}

func parseMaps(src fs.FS, mapsName, ruName string) (map[string]GameMap, error) {
	b, err := fs.ReadFile(src, mapsName)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Data struct {
			Maps map[string]rawMap `json:"maps"`
			rawRefs
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	tr := loadTranslations(src, ruName)
	out := map[string]GameMap{}
	for _, m := range raw.Data.Maps {
		gm := GameMap{ID: m.ID, NormalizedName: m.NormalizedName, Name: tr(m.Name), ScenePath: m.ScenePath, RaidMinutes: m.RaidDuration}
		var switchIDs []string // switch выхода, если это id рубильника
		for _, e := range m.Extracts {
			x := Extract{Name: tr(e.Name), Faction: e.Faction, Pos: e.Position, Switch: switchFlag(e.Switch),
				Outline: e.Outline, Switches: e.Switches}
			if e.TransferItem != nil {
				x.Paid, x.Item, x.ItemCount = true, e.TransferItem.Item, e.TransferItem.Count
			}
			gm.Extracts = append(gm.Extracts, x)
			var id string
			_ = json.Unmarshal(e.Switch, &id)
			switchIDs = append(switchIDs, id)
		}
		dropSharedSwitches(gm.Extracts, switchIDs)
		for _, t := range m.Transits {
			gm.Transits = append(gm.Transits, Transit{Name: tr(t.Description), Pos: t.Position, Outline: t.Outline})
		}
		fillContent(&gm, m.rawContent, raw.Data.rawRefs, tr)
		out[m.ScenePath] = gm
	}
	return out, nil
}

// dropSharedSwitches убирает рубильники, указанные у каждого выхода карты (от 5 выходов): так
// в снимке выглядит ошибка данных — на Таможне у всех 28 выходов рубильник ZB-013, — а не
// условие выхода. switchIDs[i] — поле switch i-го выхода, если там id.
func dropSharedSwitches(ex []Extract, switchIDs []string) {
	if len(ex) < 5 {
		return
	}
	count := map[string]int{}
	for _, e := range ex {
		seen := map[string]bool{}
		for _, id := range e.Switches {
			if !seen[id] {
				seen[id] = true
				count[id]++
			}
		}
	}
	shared := map[string]bool{}
	for id, n := range count {
		shared[id] = n == len(ex)
	}
	for i := range ex {
		var keep []string
		for _, id := range ex[i].Switches {
			if !shared[id] {
				keep = append(keep, id)
			}
		}
		ex[i].Switches = keep
		if shared[switchIDs[i]] {
			ex[i].Switch = len(keep) > 0
		}
	}
}

// switchFlag: выход требует рубильника, если switch == true или непустой id.
func switchFlag(raw json.RawMessage) bool {
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		return b
	}
	var s string
	return json.Unmarshal(raw, &s) == nil && s != ""
}

// loadTranslations: ключ → русская строка; нет перевода — сам ключ.
func loadTranslations(src fs.FS, name string) func(string) string {
	var raw struct {
		Data map[string]any `json:"data"`
	}
	if b, err := fs.ReadFile(src, name); err == nil {
		_ = json.Unmarshal(b, &raw)
	}
	return func(k string) string {
		if v, ok := raw.Data[k].(string); ok && v != "" {
			return v
		}
		return k
	}
}
