// Package core — логика рейда: текущая карта, маршрут по скриншотам, модель вида.
package core

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/Vadimushka/tarkov-map/internal/data"
	"github.com/Vadimushka/tarkov-map/internal/gamelog"
	"github.com/Vadimushka/tarkov-map/internal/mapgeo"
	"github.com/Vadimushka/tarkov-map/internal/shots"
)

type MapSource interface {
	MapByScene(scene string) (data.GameMap, bool)
	MapDef(normalizedName string) (mapgeo.MapDef, bool)
	Labels(normalizedName string) []data.Label
	ItemName(id string) (data.ItemName, bool)
	Tasks(mapID string) []data.Task
}

type Snapshot struct {
	Mode, MapID, MapName string
	Supported            bool
	Manual               bool // карта выбрана вручную, а не загружена игрой
	RaidStart, RaidEnd   time.Time
	RaidMinutes          int    // длительность рейда на карте игры; 0 — неизвестна
	LastShot             string // "21:40" или ""
	Message              string
}

type Marker struct {
	Kind          string // "extract" | "transit" | "shot"
	Name, Faction string
	Paid, Switch  bool
	PX, PY        float64
	Layer         string
	DirX, DirY    float64      // shot: единичный вектор взгляда в координатах карты
	Time          string       // shot
	Outline       [][2]float64 // extract, transit: зона в координатах карты
	Need          string       // extract: плата за выход — «Рубли × 20 000»
	Switches      []string     // extract: id рубильников, которые его открывают
	Dist          int          // extract, transit: метры по горизонтали от последнего скрина идущего рейда; -1 — неизвестно
}

// LayerInfo — этаж; первым идёт базовый слой с пустым Name.
type LayerInfo struct{ ID, Name string }

type MapView struct {
	MapID, Name string
	Supported   bool
	Rect        mapgeo.Rect
	Layers      []LayerInfo
	Markers     []Marker
	ContentGen  int // меняется, когда метки карты (Content) могли измениться
}

// mapState — карта по сцене; supported — для неё есть SVG.
type mapState struct {
	scene     string
	gm        data.GameMap
	known     bool
	def       mapgeo.MapDef
	supported bool
}

type Core struct {
	mu        sync.Mutex
	src       MapSource
	mode      string
	game      mapState // карта, которую загрузила игра: к ней относятся рейд и скрины
	manual    mapState // выбранная вручную; пустая сцена — на экране карта игры
	raidStart time.Time
	raidEnd   time.Time // после него скрины — из убежища или меню
	route     []shots.Shot
	keys      map[string]bool
	held      []heldShot // скрины, пришедшие раньше прочитанной строки GameStarted
	gen       int        // поколение меток: растёт на каждой смене карты или данных
}

type heldShot struct {
	s  shots.Shot
	at time.Time
}

func New(src MapSource) *Core {
	return &Core{src: src, keys: map[string]bool{}}
}

func (c *Core) OnLogEvent(e gamelog.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch e.Kind {
	case gamelog.EvMode:
		c.mode = e.Mode
	case gamelog.EvMap:
		c.game.scene = e.Bundle
		c.manual = mapState{} // в рейде нужна карта рейда
		c.resolve()
		c.route, c.keys, c.held = nil, map[string]bool{}, nil
		c.raidStart, c.raidEnd = time.Time{}, time.Time{}
	case gamelog.EvRaidStart:
		c.raidStart, c.raidEnd = e.At, time.Time{}
		for _, h := range c.held {
			c.add(h.s, h.at)
		}
		c.held = nil
	case gamelog.EvRaidEnd:
		if !c.raidStart.IsZero() { // при входе в игру строка бывает и до первой карты
			c.raidEnd = e.At
		}
	}
}

// OnShot добавляет точку маршрута; true — маршрут изменился.
func (c *Core) OnShot(name string, at time.Time) bool {
	s, ok := shots.ParseName(name)
	if !ok {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.game.supported {
		return false
	}
	if c.raidStart.IsZero() {
		// карта загружена, а строка о старте ещё не прочитана: лог читается раз в секунду,
		// а первый скрин делают через пару секунд после спавна
		c.held = append(c.held, heldShot{s, at})
		return false
	}
	return c.add(s, at)
}

// add принимает скрин, сделанный в текущем рейде.
func (c *Core) add(s shots.Shot, at time.Time) bool {
	if at.Before(c.raidStart) || (!c.raidEnd.IsZero() && !at.Before(c.raidEnd)) || c.keys[s.Key()] {
		return false
	}
	c.keys[s.Key()] = true
	c.route = append(c.route, s)
	return true
}

// SetSource — после смены режима данных; маршрут сохраняется.
func (c *Core) SetSource(src MapSource) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.src = src
	c.resolve()
}

// SelectMap показывает карту по сцене вместо карты игры; "" — снова как в игре.
// Рейд не меняется; выбор живёт до загрузки игрой следующей карты.
func (c *Core) SelectMap(scene string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.manual = mapState{scene: scene}
	c.resolve()
}

func (c *Core) resolve() {
	c.gen++
	c.game = c.lookup(c.game.scene)
	c.manual = c.lookup(c.manual.scene)
}

func (c *Core) lookup(scene string) mapState {
	m := mapState{scene: scene}
	if scene == "" || c.src == nil {
		return m
	}
	m.gm, m.known = c.src.MapByScene(scene)
	if m.known {
		m.def, m.supported = c.src.MapDef(m.gm.NormalizedName)
	}
	return m
}

// shown — карта на экране.
func (c *Core) shown() mapState {
	if c.manual.scene != "" {
		return c.manual
	}
	return c.game
}

func (c *Core) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	m := c.shown()
	s := Snapshot{Mode: c.mode, MapID: m.gm.NormalizedName, MapName: m.gm.Name, Supported: m.supported,
		Manual: c.manual.scene != "", RaidStart: c.raidStart, RaidEnd: c.raidEnd, RaidMinutes: c.game.gm.RaidMinutes}
	if n := len(c.route); n > 0 {
		s.LastShot = c.route[n-1].Time
	}
	switch {
	case m.scene == "":
		s.Message = "Ждём загрузки карты в игре — или выберите карту в списке справа вверху"
	case !m.known:
		s.Message = fmt.Sprintf("Неизвестная карта: %s", m.scene)
	case !m.supported:
		s.Message = fmt.Sprintf("Карта «%s» пока не поддерживается", m.gm.Name)
	}
	return s
}

func (c *Core) View() MapView {
	c.mu.Lock()
	defer c.mu.Unlock()
	m := c.shown()
	// пустые списки, а не nil: фронтенд перебирает их без проверок
	v := MapView{MapID: m.gm.NormalizedName, Name: m.gm.Name, Supported: m.supported, Layers: []LayerInfo{}, Markers: []Marker{},
		ContentGen: c.gen}
	if !m.supported {
		return v
	}
	d := m.def
	v.Rect = mapgeo.ImageRect(d)
	v.Layers = append(v.Layers, LayerInfo{ID: d.SVGLayer})
	for _, l := range d.Layers {
		v.Layers = append(v.Layers, LayerInfo{ID: l.SVGLayer, Name: l.Name})
	}
	own := m.scene == c.game.scene // маршрут рейда — только на карте рейда
	dist := func(p mapgeo.Vec3) int {
		if !own || len(c.route) == 0 || !c.raidEnd.IsZero() { // после рейда игрок в меню или убежище
			return -1
		}
		s := c.route[len(c.route)-1].Pos
		return int(math.Round(math.Hypot(p.X-s.X, p.Z-s.Z)))
	}
	for _, e := range m.gm.Extracts {
		px, py := mapgeo.Project(d, e.Pos.X, e.Pos.Z)
		v.Markers = append(v.Markers, Marker{Kind: "extract", Name: e.Name, Faction: e.Faction, Paid: e.Paid, Switch: e.Switch,
			PX: px, PY: py, Layer: mapgeo.LayerOf(d, e.Pos), Outline: outline(d, e.Outline), Need: c.price(e.Item, e.ItemCount),
			Switches: orEmpty(e.Switches), Dist: dist(e.Pos)})
	}
	for _, t := range m.gm.Transits {
		px, py := mapgeo.Project(d, t.Pos.X, t.Pos.Z)
		v.Markers = append(v.Markers, Marker{Kind: "transit", Name: t.Name, PX: px, PY: py, Layer: mapgeo.LayerOf(d, t.Pos),
			Outline: outline(d, t.Outline), Switches: []string{}, Dist: dist(t.Pos)})
	}
	if !own {
		return v
	}
	for _, s := range c.route {
		px, py := mapgeo.Project(d, s.Pos.X, s.Pos.Z)
		fx, fz := mapgeo.Forward(s.Q[0], s.Q[1], s.Q[2], s.Q[3])
		ax, ay := mapgeo.Project(d, s.Pos.X+fx, s.Pos.Z+fz)
		dx, dy := ax-px, ay-py
		if n := math.Hypot(dx, dy); n > 0 {
			dx, dy = dx/n, dy/n
		}
		v.Markers = append(v.Markers, Marker{Kind: "shot", Name: s.Name, PX: px, PY: py, Layer: mapgeo.LayerOf(d, s.Pos),
			DirX: dx, DirY: dy, Time: s.Time, Outline: [][2]float64{}, Switches: []string{}})
	}
	return v
}

// price — плата за выход по-русски: «Рубли × 20 000»; без платы — "".
func (c *Core) price(item string, count int) string {
	if item == "" {
		return ""
	}
	name := item
	if n, ok := c.src.ItemName(item); ok && n.Name != "" {
		name = n.Name
	}
	if count <= 1 {
		return name
	}
	return name + " × " + groupDigits(count)
}

// groupDigits: 20000 → «20 000» (неразрывный пробел не нужен — подпись в одну строку).
func groupDigits(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + " " + s[i:]
	}
	return s
}
