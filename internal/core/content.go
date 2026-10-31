package core

import (
	"github.com/Vadimushka/tarkov-map/internal/data"
	"github.com/Vadimushka/tarkov-map/internal/mapgeo"
)

// Pt — точка в координатах карты и её этаж.
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
	Group, Type, Name string
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

// NamedPt — пулемёт или остановка БТР.
type NamedPt struct {
	Pt
	Name string
}

// SwitchPt — рубильник; Activates — id того, что он открывает (другие рубильники, двери).
type SwitchPt struct {
	Pt
	ID, Name  string
	Activates []string
}

// SpawnPt — точка появления; Kind: pmc | scav | sniper.
type SpawnPt struct {
	Pt
	Kind string
}

type HazardArea struct {
	Pt
	Kind    string
	Outline [][2]float64 // контур в координатах карты
}

// BossPt — одна известная точка появления босса.
type BossPt struct {
	Pt
	Name               string
	Chance, SpotChance float64
}

// TaskInfo — задание на этой карте; цели — в координатах карты.
type TaskInfo struct {
	ID, Name, Trader string
	Kappa            bool
	Objectives       []ObjectiveInfo
}

type ObjectiveInfo struct {
	ID, Text string
	Optional bool
	Zones    []ZoneArea // куда прийти; Pt — центр зоны
	Points   []Pt       // где может лежать предмет задания
	Item     string
	Keys     []string
}

type ZoneArea struct {
	Pt
	Outline [][2]float64
}

// MapContent — метки вкладки «Лут и места». Отдаётся отдельно от MapView: вид
// перезапрашивается на каждый скрин, а меток на Улицах ≈2500.
type MapContent struct {
	MapID        string
	Gen          int
	Places       []PlacePt
	Containers   []ContainerPt
	Loot         []LootPt
	Items        map[string]data.ItemName // только предметы, встречающиеся в Loot этой карты
	ItemsMissing bool                     // на карте есть лут, но ни одного названия
	Locks        []LockPt
	Switches     []SwitchPt
	Hazards      []HazardArea
	Weapons      []NamedPt
	Bosses       []BossPt
	Spawns       []SpawnPt
	BTR          []NamedPt
	Tasks        []TaskInfo
}

func (c *Core) Content() MapContent {
	c.mu.Lock()
	defer c.mu.Unlock()
	g := c.shown()
	// пустые списки, а не nil: фронтенд перебирает их без проверок
	m := MapContent{MapID: g.gm.NormalizedName, Gen: c.gen, Places: []PlacePt{}, Containers: []ContainerPt{},
		Loot: []LootPt{}, Items: map[string]data.ItemName{}, Locks: []LockPt{}, Switches: []SwitchPt{},
		Hazards: []HazardArea{}, Weapons: []NamedPt{}, Bosses: []BossPt{}, Spawns: []SpawnPt{}, BTR: []NamedPt{}, Tasks: []TaskInfo{}}
	if !g.supported {
		return m
	}
	d := g.def
	at := func(p mapgeo.Vec3) Pt {
		px, py := mapgeo.Project(d, p.X, p.Z)
		return Pt{PX: px, PY: py, Layer: mapgeo.LayerOf(d, p)}
	}
	for _, l := range c.src.Labels(g.gm.NormalizedName) {
		px, py := mapgeo.Project(d, l.X, l.Z)
		layer := d.SVGLayer
		if l.Bottom != nil && l.Top != nil {
			layer = mapgeo.LayerOfSpan(d, l.X, l.Z, *l.Bottom, *l.Top)
		}
		m.Places = append(m.Places, PlacePt{Pt: Pt{PX: px, PY: py, Layer: layer}, Text: l.Text})
	}
	for _, ct := range g.gm.Containers {
		m.Containers = append(m.Containers, ContainerPt{Pt: at(ct.Pos), Group: ct.Group, Type: ct.Type, Name: ct.Name})
	}
	for _, l := range g.gm.Loose {
		m.Loot = append(m.Loot, LootPt{Pt: at(l.Pos), Items: l.Items})
		for _, id := range l.Items {
			if n, ok := c.src.ItemName(id); ok {
				m.Items[id] = n
			}
		}
	}
	m.ItemsMissing = len(m.Loot) > 0 && len(m.Items) == 0
	for _, l := range g.gm.Locks {
		key, _ := c.src.ItemName(l.KeyID)
		m.Locks = append(m.Locks, LockPt{Pt: at(l.Pos), Kind: l.Kind, Key: key.Name, NeedsPower: l.NeedsPower})
	}
	for _, s := range g.gm.Switches {
		m.Switches = append(m.Switches, SwitchPt{Pt: at(s.Pos), ID: s.ID, Name: s.Name, Activates: orEmpty(s.Activates)})
	}
	for _, h := range g.gm.Hazards {
		m.Hazards = append(m.Hazards, HazardArea{Pt: at(h.Pos), Kind: h.Kind, Outline: outline(d, h.Outline)})
	}
	for _, s := range g.gm.Spawns {
		m.Spawns = append(m.Spawns, SpawnPt{Pt: at(s.Pos), Kind: s.Kind})
	}
	for _, b := range g.gm.BTRStops {
		m.BTR = append(m.BTR, NamedPt{Pt: at(b.Pos), Name: b.Name})
	}
	for _, w := range g.gm.Weapons {
		m.Weapons = append(m.Weapons, NamedPt{Pt: at(w.Pos), Name: w.Name})
	}
	for _, b := range g.gm.Bosses {
		for _, s := range b.Spots {
			m.Bosses = append(m.Bosses, BossPt{Pt: at(s.Pos), Name: b.Name, Chance: b.Chance, SpotChance: s.Chance})
		}
	}
	for _, t := range c.src.Tasks(g.gm.ID) {
		ti := TaskInfo{ID: t.ID, Name: t.Name, Trader: t.Trader, Kappa: t.Kappa, Objectives: []ObjectiveInfo{}}
		for _, o := range t.Objectives {
			oi := ObjectiveInfo{ID: o.ID, Text: o.Text, Optional: o.Optional, Zones: []ZoneArea{}, Points: []Pt{},
				Item: o.Item, Keys: orEmpty(o.Keys)}
			for _, z := range o.Zones {
				px, py := mapgeo.Project(d, z.Pos.X, z.Pos.Z)
				layer := mapgeo.LayerOfSpan(d, z.Pos.X, z.Pos.Z, z.Bottom, z.Top)
				oi.Zones = append(oi.Zones, ZoneArea{Pt: Pt{PX: px, PY: py, Layer: layer}, Outline: outline(d, z.Outline)})
			}
			for _, p := range o.Points {
				oi.Points = append(oi.Points, at(p))
			}
			ti.Objectives = append(ti.Objectives, oi)
		}
		m.Tasks = append(m.Tasks, ti)
	}
	return m
}

// outline — контур в координатах карты; пустой контур — [], а не null.
func outline(d mapgeo.MapDef, pts []mapgeo.Vec3) [][2]float64 {
	out := [][2]float64{}
	for _, p := range pts {
		px, py := mapgeo.Project(d, p.X, p.Z)
		out = append(out, [2]float64{px, py})
	}
	return out
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
