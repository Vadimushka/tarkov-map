package data

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/Vadimushka/tarkov-map/internal/mapgeo"
)

// Container — контейнер с лутом. Group: weapon | med | tech | safe | cache | body | misc.
type Container struct {
	Group, Name string
	Pos         mapgeo.Vec3
}

// Loose — точка, где на земле может появиться лут; Items — id предметов.
type Loose struct {
	Pos   mapgeo.Vec3
	Items []string
}

// Lock — запертая дверь или багажник. Kind — lockType из данных: door | trunk | …
type Lock struct {
	Kind       string
	KeyID      string
	NeedsPower bool
	Pos        mapgeo.Vec3
}

type Switch struct {
	Name string
	Pos  mapgeo.Vec3
}

// Hazard — опасная зона. Kind — hazardType: minefield | sniper | hazard | …
type Hazard struct {
	Kind    string
	Pos     mapgeo.Vec3
	Outline []mapgeo.Vec3
}

// Weapon — станковый пулемёт или гранатомёт.
type Weapon struct {
	Name string
	Pos  mapgeo.Vec3
}

// BossSpot — известная точка появления; Chance — шанс зоны, если босс появился.
type BossSpot struct {
	Chance float64
	Pos    mapgeo.Vec3
}

// Boss — босс карты; Chance — шанс появления в рейде.
type Boss struct {
	Name   string
	Chance float64
	Spots  []BossSpot
}

var containerGroups = map[string]string{
	"weapon-box": "weapon", "grenade-box": "weapon", "wooden-ammo-box": "weapon",
	"medbag": "med", "medcase": "med", "medical-supply-crate": "med",
	"pc-block": "tech", "toolbox": "tech", "technical-supply-crate": "tech",
	"safe": "safe", "bank-safe": "safe", "cash-register": "safe", "bank-cash-register": "safe",
	"buried-barrel-cache": "cache", "ground-cache": "cache", "shturmans-stash": "cache",
	"dead-scav": "body", "scav-body": "body", "pmc-body": "body", "civilian-body": "body", "lab-technician-body": "body",
}

// containerGroup: незнакомый после обновления игры тип попадает в «Сумки и прочее».
func containerGroup(normalizedName string) string {
	if g, ok := containerGroups[normalizedName]; ok {
		return g
	}
	return "misc"
}

// --- разбор меток из json.tarkov.dev/{mode}/maps ---

// rawRef — запись верхнеуровневых словарей data.lootContainers, data.stationaryWeapons, data.mobs.
type rawRef struct {
	Name           string `json:"name"`
	NormalizedName string `json:"normalizedName"`
}

type rawRefs struct {
	LootContainers    map[string]rawRef `json:"lootContainers"`
	StationaryWeapons map[string]rawRef `json:"stationaryWeapons"`
	Mobs              map[string]rawRef `json:"mobs"`
}

type rawContent struct {
	LootContainers []struct {
		LootContainer string      `json:"lootContainer"`
		Position      mapgeo.Vec3 `json:"position"`
	} `json:"lootContainers"`
	LootLoose []struct {
		Position mapgeo.Vec3 `json:"position"`
		Items    []string    `json:"items"`
	} `json:"lootLoose"`
	Locks []struct {
		LockType   string      `json:"lockType"`
		Key        string      `json:"key"`
		NeedsPower bool        `json:"needsPower"`
		Position   mapgeo.Vec3 `json:"position"`
	} `json:"locks"`
	Switches []struct {
		Name     string      `json:"name"`
		Position mapgeo.Vec3 `json:"position"`
	} `json:"switches"`
	Hazards []struct {
		HazardType string        `json:"hazardType"`
		Position   mapgeo.Vec3   `json:"position"`
		Outline    []mapgeo.Vec3 `json:"outline"`
	} `json:"hazards"`
	StationaryWeapons []struct {
		StationaryWeapon string      `json:"stationaryWeapon"`
		Position         mapgeo.Vec3 `json:"position"`
	} `json:"stationaryWeapons"`
	Bosses []struct {
		Mob            string  `json:"mob"`
		SpawnChance    float64 `json:"spawnChance"`
		SpawnLocations []struct {
			Chance    float64       `json:"chance"`
			Positions []mapgeo.Vec3 `json:"positions"`
		} `json:"spawnLocations"`
	} `json:"bosses"`
}

// fillContent переносит метки карты в gm, переводя названия через tr.
func fillContent(gm *GameMap, rc rawContent, refs rawRefs, tr func(string) string) {
	for _, c := range rc.LootContainers {
		name, group := c.LootContainer, "misc"
		if r, ok := refs.LootContainers[c.LootContainer]; ok {
			name, group = tr(r.Name), containerGroup(r.NormalizedName)
		}
		gm.Containers = append(gm.Containers, Container{Group: group, Name: name, Pos: c.Position})
	}
	for _, l := range rc.LootLoose {
		gm.Loose = append(gm.Loose, Loose{Pos: l.Position, Items: l.Items})
	}
	for _, l := range rc.Locks {
		gm.Locks = append(gm.Locks, Lock{Kind: l.LockType, KeyID: l.Key, NeedsPower: l.NeedsPower, Pos: l.Position})
	}
	for _, s := range rc.Switches {
		gm.Switches = append(gm.Switches, Switch{Name: tr(s.Name), Pos: s.Position})
	}
	for _, h := range rc.Hazards {
		gm.Hazards = append(gm.Hazards, Hazard{Kind: h.HazardType, Pos: h.Position, Outline: h.Outline})
	}
	for _, w := range rc.StationaryWeapons {
		name := w.StationaryWeapon
		if r, ok := refs.StationaryWeapons[w.StationaryWeapon]; ok {
			name = tr(r.Name)
		}
		gm.Weapons = append(gm.Weapons, Weapon{Name: name, Pos: w.Position})
	}
	for _, b := range rc.Bosses {
		if !strings.HasPrefix(b.Mob, "boss") { // ЧВК-боты, рейдеры, сектанты — не боссы
			continue
		}
		name := b.Mob
		if r, ok := refs.Mobs[b.Mob]; ok {
			name = r.Name
		}
		boss := Boss{Name: tr(name), Chance: b.SpawnChance}
		for _, l := range b.SpawnLocations {
			for _, p := range l.Positions {
				boss.Spots = append(boss.Spots, BossSpot{Chance: l.Chance, Pos: p})
			}
		}
		gm.Bosses = append(gm.Bosses, boss)
	}
}

// ItemName — название предмета по-русски: полное и краткое.
type ItemName struct{ Name, Short string }

// parseItems читает items_ru: ключи "<id> Name" и "<id> ShortName". Нет файла или он
// битый — пустой словарь: названия предметов необязательны.
func parseItems(file string) map[string]ItemName {
	out := map[string]ItemName{}
	var raw struct {
		Data map[string]any `json:"data"`
	}
	b, err := os.ReadFile(file)
	if err != nil || json.Unmarshal(b, &raw) != nil {
		return out
	}
	for k, v := range raw.Data {
		text, _ := v.(string)
		if id, ok := strings.CutSuffix(k, " Name"); ok {
			n := out[id]
			n.Name = text
			out[id] = n
		} else if id, ok := strings.CutSuffix(k, " ShortName"); ok {
			n := out[id]
			n.Short = text
			out[id] = n
		}
	}
	return out
}
