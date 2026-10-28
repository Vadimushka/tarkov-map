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
	route     []shots.Shot
	keys      map[string]bool
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
		c.route, c.keys, c.raidStart = nil, map[string]bool{}, time.Time{}
	case gamelog.EvRaidStart:
		c.raidStart = e.At
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
	if !c.supported || c.raidStart.IsZero() || at.Before(c.raidStart) || c.keys[s.Key()] {
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
	v := MapView{MapID: c.gm.NormalizedName, Name: c.gm.Name, Supported: c.supported}
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
