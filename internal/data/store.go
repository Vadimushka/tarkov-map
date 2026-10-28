// Package data скачивает и кэширует карты и точки tarkov.dev.
package data

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Vadimushka/tarkov-map/internal/mapgeo"
)

var ErrNoData = errors.New("нет данных: нет сети и нет кэша")

const ttl = 24 * time.Hour

// Extract — выход с карты. Faction: "pmc" | "scav" | "shared".
type Extract struct {
	Name, Faction string
	Pos           mapgeo.Vec3
	Paid, Switch  bool
}

type Transit struct {
	Name string
	Pos  mapgeo.Vec3
}

type GameMap struct {
	ID, NormalizedName, Name, ScenePath string
	Extracts                            []Extract
	Transits                            []Transit
}

// Варианты карт, у которых своей SVG нет.
var aliases = map[string]string{
	"night-factory":        "factory",
	"ground-zero-21":       "ground-zero",
	"ground-zero-tutorial": "ground-zero",
}

const assetsSVG = "https://assets.tarkov.dev/maps/svg"

type Store struct {
	JSONBase    string
	MapsJSONURL string
	SVGBase     string // "" — брать svgPath как есть
	SVGFallback string

	dir    string
	client *http.Client
	now    func() time.Time

	mu      sync.RWMutex
	byScene map[string]GameMap
	defs    map[string]mapgeo.MapDef
	updated time.Time
}

func NewStore(cacheDir string, client *http.Client, now func() time.Time) *Store {
	return &Store{
		JSONBase:    "https://json.tarkov.dev",
		MapsJSONURL: "https://raw.githubusercontent.com/the-hideout/tarkov-dev/HEAD/src/data/maps.json",
		SVGFallback: "https://raw.githubusercontent.com/the-hideout/tarkov-dev-svg-maps/HEAD",
		dir:         cacheDir,
		client:      client,
		now:         now,
	}
}

// Load обновляет кэш режима ("pve" | "regular"), если он старше суток или force,
// и разбирает данные. Без сети работает на кэше; без сети и без кэша — ErrNoData.
func (s *Store) Load(mode string, force bool) error {
	files := []struct{ url, name string }{
		{s.MapsJSONURL, "maps.json"},
		{s.JSONBase + "/" + mode + "/maps", mode + "_maps.json"},
		{s.JSONBase + "/" + mode + "/maps_ru", mode + "_maps_ru.json"},
	}
	for _, f := range files {
		if err := s.ensure(f.url, f.name, force); err != nil {
			return err
		}
	}
	defs, err := parseDefs(filepath.Join(s.dir, "maps.json"))
	if err != nil {
		return fmt.Errorf("maps.json: %w", err)
	}
	maps, err := parseMaps(filepath.Join(s.dir, mode+"_maps.json"), filepath.Join(s.dir, mode+"_maps_ru.json"))
	if err != nil {
		return fmt.Errorf("%s_maps: %w", mode, err)
	}
	var updated time.Time
	if st, err := os.Stat(filepath.Join(s.dir, mode+"_maps.json")); err == nil {
		updated = st.ModTime()
	}
	s.mu.Lock()
	s.defs, s.byScene, s.updated = defs, maps, updated
	s.mu.Unlock()
	return nil
}

func (s *Store) ensure(url, name string, force bool) error {
	p := filepath.Join(s.dir, name)
	st, statErr := os.Stat(p)
	if statErr == nil && !force && s.now().Sub(st.ModTime()) < ttl {
		return nil
	}
	if err := s.download(url, p); err != nil {
		if statErr == nil {
			return nil // сети нет — живём на кэше
		}
		return fmt.Errorf("%w (%s: %v)", ErrNoData, name, err)
	}
	return nil
}

func (s *Store) download(url, dst string) error {
	resp, err := s.client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	now := s.now()
	return os.Chtimes(dst, now, now)
}

func (s *Store) MapByScene(scenePath string) (GameMap, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.byScene[scenePath]
	return m, ok
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

func (s *Store) Updated() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.updated
}

// SVG отдаёт текст SVG-карты из кэша, при необходимости скачивая его.
func (s *Store) SVG(def mapgeo.MapDef) (string, error) {
	file := path.Base(def.SVGPath)
	p := filepath.Join(s.dir, "svg", file)
	if b, err := os.ReadFile(p); err == nil {
		return string(b), nil
	}
	primary := def.SVGPath
	if s.SVGBase != "" {
		primary = strings.Replace(primary, assetsSVG, s.SVGBase, 1)
	}
	if err := s.download(primary, p); err != nil {
		if err2 := s.download(s.SVGFallback+"/"+file, p); err2 != nil {
			return "", fmt.Errorf("SVG %s: %v; запасной адрес: %v", file, err, err2)
		}
	}
	b, err := os.ReadFile(p)
	return string(b), err
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
	Layers             []struct {
		Name     string `json:"name"`
		SVGLayer string `json:"svgLayer"`
		Extents  []struct {
			Height [2]float64        `json:"height"`
			Bounds []json.RawMessage `json:"bounds"` // [[x,z],[x,z],"имя"]
		} `json:"extents"`
	} `json:"layers"`
}

func parseDefs(file string) (map[string]mapgeo.MapDef, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		NormalizedName string   `json:"normalizedName"`
		Maps           []rawDef `json:"maps"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := map[string]mapgeo.MapDef{}
	for _, m := range raw {
		for _, d := range m.Maps {
			if d.Projection != "interactive" || d.SVGPath == "" {
				continue
			}
			out[m.NormalizedName] = toDef(d)
			break
		}
	}
	return out, nil
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
	Extracts       []struct {
		Name         string          `json:"name"`
		Faction      string          `json:"faction"`
		Switch       json.RawMessage `json:"switch"` // bool или id рубильника
		Position     mapgeo.Vec3     `json:"position"`
		TransferItem json.RawMessage `json:"transferItem"`
	} `json:"extracts"`
	Transits []struct {
		Description string      `json:"description"`
		Position    mapgeo.Vec3 `json:"position"`
	} `json:"transits"`
}

func parseMaps(mapsFile, ruFile string) (map[string]GameMap, error) {
	b, err := os.ReadFile(mapsFile)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Data struct {
			Maps map[string]rawMap `json:"maps"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	tr := loadTranslations(ruFile)
	out := map[string]GameMap{}
	for _, m := range raw.Data.Maps {
		gm := GameMap{ID: m.ID, NormalizedName: m.NormalizedName, Name: tr(m.Name), ScenePath: m.ScenePath}
		for _, e := range m.Extracts {
			paid := len(e.TransferItem) > 0 && string(e.TransferItem) != "null"
			gm.Extracts = append(gm.Extracts, Extract{Name: tr(e.Name), Faction: e.Faction, Pos: e.Position, Paid: paid, Switch: switchFlag(e.Switch)})
		}
		for _, t := range m.Transits {
			gm.Transits = append(gm.Transits, Transit{Name: tr(t.Description), Pos: t.Position})
		}
		out[m.ScenePath] = gm
	}
	return out, nil
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
func loadTranslations(file string) func(string) string {
	var raw struct {
		Data map[string]any `json:"data"`
	}
	if b, err := os.ReadFile(file); err == nil {
		_ = json.Unmarshal(b, &raw)
	}
	return func(k string) string {
		if v, ok := raw.Data[k].(string); ok && v != "" {
			return v
		}
		return k
	}
}
