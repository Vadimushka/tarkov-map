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

// NamedPt — рубильник или пулемёт.
type NamedPt struct {
	Pt
	Name string
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
	Switches     []NamedPt
	Hazards      []HazardArea
	Weapons      []NamedPt
	Bosses       []BossPt
}

func (c *Core) Content() MapContent {
	c.mu.Lock()
	defer c.mu.Unlock()
	// пустые списки, а не nil: фронтенд перебирает их без проверок
	m := MapContent{MapID: c.gm.NormalizedName, Gen: c.gen, Places: []PlacePt{}, Containers: []ContainerPt{},
		Loot: []LootPt{}, Items: map[string]data.ItemName{}, Locks: []LockPt{}, Switches: []NamedPt{},
		Hazards: []HazardArea{}, Weapons: []NamedPt{}, Bosses: []BossPt{}}
	if !c.supported {
		return m
	}
	d := c.def
	at := func(p mapgeo.Vec3) Pt {
		px, py := mapgeo.Project(d, p.X, p.Z)
		return Pt{PX: px, PY: py, Layer: mapgeo.LayerOf(d, p)}
	}
	for _, l := range c.src.Labels(c.gm.NormalizedName) {
		px, py := mapgeo.Project(d, l.X, l.Z)
		layer := d.SVGLayer
		if l.Bottom != nil && l.Top != nil {
			layer = mapgeo.LayerOfSpan(d, l.X, l.Z, *l.Bottom, *l.Top)
		}
		m.Places = append(m.Places, PlacePt{Pt: Pt{PX: px, PY: py, Layer: layer}, Text: l.Text})
	}
	for _, ct := range c.gm.Containers {
		m.Containers = append(m.Containers, ContainerPt{Pt: at(ct.Pos), Group: ct.Group, Name: ct.Name})
	}
	for _, l := range c.gm.Loose {
		m.Loot = append(m.Loot, LootPt{Pt: at(l.Pos), Items: l.Items})
		for _, id := range l.Items {
			if n, ok := c.src.ItemName(id); ok {
				m.Items[id] = n
			}
		}
	}
	m.ItemsMissing = len(m.Loot) > 0 && len(m.Items) == 0
	for _, l := range c.gm.Locks {
		key, _ := c.src.ItemName(l.KeyID)
		m.Locks = append(m.Locks, LockPt{Pt: at(l.Pos), Kind: l.Kind, Key: key.Name, NeedsPower: l.NeedsPower})
	}
	for _, s := range c.gm.Switches {
		m.Switches = append(m.Switches, NamedPt{Pt: at(s.Pos), Name: s.Name})
	}
	for _, h := range c.gm.Hazards {
		a := HazardArea{Pt: at(h.Pos), Kind: h.Kind, Outline: [][2]float64{}}
		for _, p := range h.Outline {
			px, py := mapgeo.Project(d, p.X, p.Z)
			a.Outline = append(a.Outline, [2]float64{px, py})
		}
		m.Hazards = append(m.Hazards, a)
	}
	for _, w := range c.gm.Weapons {
		m.Weapons = append(m.Weapons, NamedPt{Pt: at(w.Pos), Name: w.Name})
	}
	for _, b := range c.gm.Bosses {
		for _, s := range b.Spots {
			m.Bosses = append(m.Bosses, BossPt{Pt: at(s.Pos), Name: b.Name, Chance: b.Chance, SpotChance: s.Chance})
		}
	}
	return m
}
