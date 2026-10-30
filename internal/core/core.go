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
}

type Snapshot struct {
	Mode, MapID, MapName string
	Supported            bool
	RaidStart            time.Time
	LastShot             string // "21:40" или ""
	Message              string
}

type Marker struct {
	Kind          string // "extract" | "transit" | "shot"
	Name, Faction string
	Paid, Switch  bool
	PX, PY        float64
	Layer         string
	DirX, DirY    float64 // shot: единичный вектор взгляда в координатах карты
	Time          string  // shot
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

type Core struct {
	mu        sync.Mutex
	src       MapSource
	mode      string
	scene     string
	gm        data.GameMap
	known     bool
	def       mapgeo.MapDef
	supported bool
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
		c.scene = e.Bundle
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
	if !c.supported {
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

func (c *Core) resolve() {
	c.gen++
	c.gm, c.known = data.GameMap{}, false
	c.def, c.supported = mapgeo.MapDef{}, false
	if c.scene == "" || c.src == nil {
		return
	}
	c.gm, c.known = c.src.MapByScene(c.scene)
	if c.known {
		c.def, c.supported = c.src.MapDef(c.gm.NormalizedName)
	}
}

func (c *Core) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := Snapshot{Mode: c.mode, MapID: c.gm.NormalizedName, MapName: c.gm.Name, Supported: c.supported, RaidStart: c.raidStart}
	if n := len(c.route); n > 0 {
		s.LastShot = c.route[n-1].Time
	}
	switch {
	case c.scene == "":
		s.Message = "Ждём загрузки карты в игре"
	case !c.known:
		s.Message = fmt.Sprintf("Неизвестная карта: %s", c.scene)
	case !c.supported:
		s.Message = fmt.Sprintf("Карта «%s» пока не поддерживается", c.gm.Name)
	}
	return s
}

func (c *Core) View() MapView {
	c.mu.Lock()
	defer c.mu.Unlock()
	// пустые списки, а не nil: фронтенд перебирает их без проверок
	v := MapView{MapID: c.gm.NormalizedName, Name: c.gm.Name, Supported: c.supported, Layers: []LayerInfo{}, Markers: []Marker{}, ContentGen: c.gen}
	if !c.supported {
		return v
	}
	d := c.def
	v.Rect = mapgeo.ImageRect(d)
	v.Layers = append(v.Layers, LayerInfo{ID: d.SVGLayer})
	for _, l := range d.Layers {
		v.Layers = append(v.Layers, LayerInfo{ID: l.SVGLayer, Name: l.Name})
	}
	for _, e := range c.gm.Extracts {
		px, py := mapgeo.Project(d, e.Pos.X, e.Pos.Z)
		v.Markers = append(v.Markers, Marker{Kind: "extract", Name: e.Name, Faction: e.Faction, Paid: e.Paid, Switch: e.Switch,
			PX: px, PY: py, Layer: mapgeo.LayerOf(d, e.Pos)})
	}
	for _, t := range c.gm.Transits {
		px, py := mapgeo.Project(d, t.Pos.X, t.Pos.Z)
		v.Markers = append(v.Markers, Marker{Kind: "transit", Name: t.Name, PX: px, PY: py, Layer: mapgeo.LayerOf(d, t.Pos)})
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
			DirX: dx, DirY: dy, Time: s.Time})
	}
	return v
}
