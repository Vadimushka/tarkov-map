// Package data скачивает и кэширует карты и точки tarkov.dev.
package data

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
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
	Containers                          []Container
	Loose                               []Loose
	Locks                               []Lock
	Switches                            []Switch
	Hazards                             []Hazard
	Weapons                             []Weapon
	Bosses                              []Boss
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
	// Seed — снимок данных, встроенный в exe, с путями как в кэше (maps.json, svg/X.svg);
	// nil — без снимка. Без сети приложение работает на нём с первого запуска.
	Seed fs.FS

	dir    string
	client *http.Client
	now    func() time.Time

	mu      sync.RWMutex
	byScene map[string]GameMap
	defs    map[string]mapgeo.MapDef
	labels  map[string][]Label
	items   map[string]ItemName
	updated time.Time

	itemsFailed map[string]bool // режимы, для которых items_ru не скачался в этом запуске (под mu)

	fileLocks  sync.Map // путь файла → *sync.Mutex закачки
	svgRefresh sync.Map // путь SVG → идёт фоновое обновление
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

type remoteFile struct{ url, name string }

func (s *Store) files(mode string) []remoteFile {
	return []remoteFile{
		{s.MapsJSONURL, "maps.json"},
		{s.JSONBase + "/" + mode + "/maps", mode + "_maps.json"},
		{s.JSONBase + "/" + mode + "/maps_ru", mode + "_maps_ru.json"},
	}
}

// itemsFile — названия предметов. Файл необязателен и в files не входит: без него
// карта работает, недоступен только поиск лута.
func (s *Store) itemsFile(mode string) remoteFile {
	return remoteFile{s.JSONBase + "/" + mode + "/items_ru", mode + "_items_ru.json"}
}

// Load обновляет кэш режима ("pve" | "regular"), если он старше суток или force,
// и разбирает данные. Без сети работает на кэше; без сети и без кэша — ErrNoData.
func (s *Store) Load(mode string, force bool) error {
	for _, f := range s.files(mode) {
		if err := s.ensure(f.url, f.name, force); err != nil {
			return err
		}
	}
	it := s.itemsFile(mode)
	_ = s.ensure(it.url, it.name, force)
	return s.parse(mode)
}

// LoadCached разбирает кэш режима любой давности, не обращаясь к сети; нет файлов — ErrNoData.
// Недостающее или более старое, чем снимок, сначала берётся из снимка.
func (s *Store) LoadCached(mode string) error {
	s.seed(s.itemsFile(mode).name)
	for _, f := range s.files(mode) {
		s.seed(f.name)
		if _, err := os.Stat(filepath.Join(s.dir, f.name)); err != nil {
			return fmt.Errorf("%w (%s)", ErrNoData, f.name)
		}
	}
	return s.parse(mode)
}

// Stale — какого-то файла режима нет или он старше суток. Названия предметов учитываются,
// только пока их не пытались скачать в этом запуске: иначе при недоступном items_ru каждая
// загрузка карты заново качала бы все файлы режима.
func (s *Store) Stale(mode string) bool {
	for _, f := range s.files(mode) {
		st, err := os.Stat(filepath.Join(s.dir, f.name))
		if err != nil || s.now().Sub(st.ModTime()) >= ttl {
			return true
		}
	}
	s.mu.RLock()
	failed := s.itemsFailed[mode]
	s.mu.RUnlock()
	if failed {
		return false
	}
	st, err := os.Stat(filepath.Join(s.dir, s.itemsFile(mode).name))
	return err != nil || s.now().Sub(st.ModTime()) >= ttl
}

// Fetch скачивает файлы режима, не трогая данные в памяти: все при force, иначе только
// отсутствующие и устаревшие. Ошибка — сеть не ответила; уже лежащие файлы при этом
// остаются прежними. Неудача с названиями предметов ошибкой не считается, а только
// запоминается (см. Stale).
func (s *Store) Fetch(mode string, force bool) error {
	for _, f := range s.files(mode) {
		if !force && s.fresh(f.name) {
			continue
		}
		if err := s.download(f.url, filepath.Join(s.dir, f.name)); err != nil {
			return fmt.Errorf("%s: %w", f.name, err)
		}
	}
	it := s.itemsFile(mode)
	if !force && s.fresh(it.name) {
		return nil
	}
	failed := s.download(it.url, filepath.Join(s.dir, it.name)) != nil
	s.mu.Lock()
	if s.itemsFailed == nil {
		s.itemsFailed = map[string]bool{}
	}
	s.itemsFailed[mode] = failed
	s.mu.Unlock()
	return nil
}

func (s *Store) parse(mode string) error {
	defs, labels, err := parseDefs(filepath.Join(s.dir, "maps.json"))
	if err != nil {
		return fmt.Errorf("maps.json: %w", err)
	}
	maps, err := parseMaps(filepath.Join(s.dir, mode+"_maps.json"), filepath.Join(s.dir, mode+"_maps_ru.json"))
	if err != nil {
		return fmt.Errorf("%s_maps: %w", mode, err)
	}
	items := parseItems(filepath.Join(s.dir, s.itemsFile(mode).name))
	var updated time.Time
	if st, err := os.Stat(filepath.Join(s.dir, mode+"_maps.json")); err == nil {
		updated = st.ModTime()
	}
	s.mu.Lock()
	s.defs, s.labels, s.items, s.byScene, s.updated = defs, labels, items, maps, updated
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

// fresh — файл кэша есть и моложе суток.
func (s *Store) fresh(name string) bool {
	st, err := os.Stat(filepath.Join(s.dir, name))
	return err == nil && s.now().Sub(st.ModTime()) < ttl
}

// seed подкладывает в кэш файл из снимка, если в кэше его нет или он старше снимка.
// Время файла — время снимка: устаревший снимок обновится из сети, как обычный кэш.
// Битый снимок не мешает: файл остаётся каким был.
func (s *Store) seed(name string) {
	if s.Seed == nil {
		return
	}
	src, err := fs.Stat(s.Seed, name)
	if err != nil {
		return
	}
	dst := filepath.Join(s.dir, filepath.FromSlash(name))
	l, _ := s.fileLocks.LoadOrStore(dst, &sync.Mutex{})
	l.(*sync.Mutex).Lock()
	defer l.(*sync.Mutex).Unlock()
	if cur, err := os.Stat(dst); err == nil && !cur.ModTime().Before(src.ModTime()) {
		return
	}
	b, err := fs.ReadFile(s.Seed, name)
	if err != nil || os.MkdirAll(filepath.Dir(dst), 0o755) != nil {
		return
	}
	// своё имя временного файла: кэш может делить вторая копия приложения
	tmp, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".*.seed")
	if err != nil {
		return
	}
	_, werr := tmp.Write(b)
	if cerr := tmp.Close(); werr != nil || cerr != nil || os.Chtimes(tmp.Name(), src.ModTime(), src.ModTime()) != nil ||
		os.Rename(tmp.Name(), dst) != nil {
		os.Remove(tmp.Name())
	}
}

// download атомарно заменяет dst; закачки одного файла идут по очереди (общий .tmp).
func (s *Store) download(url, dst string) error {
	l, _ := s.fileLocks.LoadOrStore(dst, &sync.Mutex{})
	l.(*sync.Mutex).Lock()
	defer l.(*sync.Mutex).Unlock()
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

// Labels — подписи мест SVG-карты (варианты карт без своей SVG — по aliases).
func (s *Store) Labels(normalizedName string) []Label {
	if a, ok := aliases[normalizedName]; ok {
		normalizedName = a
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.labels[normalizedName]
}

func (s *Store) Updated() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.updated
}

// SVG отдаёт текст SVG-карты из кэша, при необходимости скачивая его. Устаревший
// (старше суток) файл отдаётся сразу и обновляется в фоне — к следующему открытию карты.
func (s *Store) SVG(def mapgeo.MapDef) (string, error) {
	p := s.svgFile(def)
	s.seed("svg/" + filepath.Base(p))
	if b, err := os.ReadFile(p); err == nil {
		if st, err := os.Stat(p); err == nil && s.now().Sub(st.ModTime()) >= ttl {
			if _, busy := s.svgRefresh.LoadOrStore(p, true); !busy {
				go func() {
					defer s.svgRefresh.Delete(p)
					_ = s.RefreshSVG(def)
				}()
			}
		}
		return string(b), nil
	}
	if err := s.RefreshSVG(def); err != nil {
		return "", err
	}
	b, err := os.ReadFile(p)
	return string(b), err
}

// RefreshSVG перекачивает SVG-карту (основной адрес, затем запасной); при неудаче
// прежний файл остаётся.
func (s *Store) RefreshSVG(def mapgeo.MapDef) error {
	p := s.svgFile(def)
	primary := def.SVGPath
	if s.SVGBase != "" {
		primary = strings.Replace(primary, assetsSVG, s.SVGBase, 1)
	}
	if err := s.download(primary, p); err != nil {
		if err2 := s.download(s.SVGFallback+"/"+filepath.Base(p), p); err2 != nil {
			return fmt.Errorf("SVG %s: %v; запасной адрес: %v", filepath.Base(p), err, err2)
		}
	}
	return nil
}

func (s *Store) svgFile(def mapgeo.MapDef) string {
	return filepath.Join(s.dir, "svg", path.Base(def.SVGPath))
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

func parseDefs(file string) (map[string]mapgeo.MapDef, map[string][]Label, error) {
	b, err := os.ReadFile(file)
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
	rawContent
}

func parseMaps(mapsFile, ruFile string) (map[string]GameMap, error) {
	b, err := os.ReadFile(mapsFile)
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
		fillContent(&gm, m.rawContent, raw.Data.rawRefs, tr)
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
