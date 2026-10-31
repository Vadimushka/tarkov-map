package data

import (
	"encoding/json"
	"io/fs"
	"log"
	"strings"

	"github.com/Vadimushka/tarkov-map/internal/mapgeo"
)

// Container — контейнер с лутом. Group: weapon | med | tech | safe | cache | body | misc;
// Type — normalizedName типа контейнера (по нему выбирается значок), пусто для незнакомого.
type Container struct {
	Group, Type, Name string
	Pos               mapgeo.Vec3
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

// Switch — рубильник; Activates — id рубильников и дверей, которые он открывает.
type Switch struct {
	ID, Name  string
	Pos       mapgeo.Vec3
	Activates []string
}

// Spawn — точка появления. Kind: pmc | scav | sniper.
type Spawn struct {
	Kind string
	Pos  mapgeo.Vec3
}

// NamedPos — подписанная точка: остановка БТР.
type NamedPos struct {
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
		ID        string      `json:"id"`
		Name      string      `json:"name"`
		Position  mapgeo.Vec3 `json:"position"`
		Activates []struct {
			Switch string `json:"switch"`
		} `json:"activates"`
	} `json:"switches"`
	Spawns []struct {
		Position   mapgeo.Vec3 `json:"position"`
		Sides      []string    `json:"sides"`
		Categories []string    `json:"categories"`
		ZoneName   string      `json:"zoneName"`
	} `json:"spawns"`
	BTRStops []struct {
		Name string  `json:"name"`
		X    float64 `json:"x"`
		Y    float64 `json:"y"`
		Z    float64 `json:"z"`
	} `json:"btrStops"`
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
		name, group, typ := c.LootContainer, "misc", ""
		if r, ok := refs.LootContainers[c.LootContainer]; ok {
			name, group, typ = tr(r.Name), containerGroup(r.NormalizedName), r.NormalizedName
		}
		gm.Containers = append(gm.Containers, Container{Group: group, Type: typ, Name: name, Pos: c.Position})
	}
	for _, l := range rc.LootLoose {
		gm.Loose = append(gm.Loose, Loose{Pos: l.Position, Items: l.Items})
	}
	for _, l := range rc.Locks {
		gm.Locks = append(gm.Locks, Lock{Kind: l.LockType, KeyID: l.Key, NeedsPower: l.NeedsPower, Pos: l.Position})
	}
	for _, s := range rc.Switches {
		sw := Switch{ID: s.ID, Name: tr(s.Name), Pos: s.Position}
		for _, a := range s.Activates {
			sw.Activates = append(sw.Activates, a.Switch)
		}
		gm.Switches = append(gm.Switches, sw)
	}
	for _, s := range rc.Spawns {
		for _, kind := range spawnKinds(s.Sides, s.Categories, s.ZoneName) {
			gm.Spawns = append(gm.Spawns, Spawn{Kind: kind, Pos: s.Position})
		}
	}
	for _, b := range rc.BTRStops {
		gm.BTRStops = append(gm.BTRStops, NamedPos{Name: tr(b.Name), Pos: mapgeo.Vec3{X: b.X, Y: b.Y, Z: b.Z}})
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

// spawnKinds: «all» — точка и ЧВК, и Диких; снайперская — с зоной «snip…» в имени (ZoneSniper,
// ZoneSnipeBrige) или с категорией sniper без player (в PvE-снимке Эпицентра у точек игроков
// бывает ["player","sniper"]). Точки только для боссов и событий (season01) пропускаются:
// боссов показывает свой слой с шансами.
func spawnKinds(sides, categories []string, zone string) []string {
	people, player, sniperCat := false, false, false
	for _, c := range categories {
		people = people || c == "player" || c == "bot" || c == "botpmc" || c == "sniper"
		player = player || c == "player"
		sniperCat = sniperCat || c == "sniper"
	}
	if !people {
		return nil
	}
	sniper := strings.Contains(strings.ToLower(zone), "snip") || (sniperCat && !player)
	var out []string
	for _, s := range sides {
		if s == "pmc" || s == "all" {
			out = append(out, "pmc")
		}
		if s == "scav" || s == "all" {
			if sniper {
				out = append(out, "sniper")
			} else {
				out = append(out, "scav")
			}
		}
	}
	return out
}

// ItemName — название предмета по-русски: полное и краткое.
type ItemName struct{ Name, Short string }

// parseItems читает items_ru: ключи "<id> Name" и "<id> ShortName". Нет файла или он
// битый — пустой словарь: названия предметов необязательны.
func parseItems(src fs.FS, name string) map[string]ItemName {
	out := map[string]ItemName{}
	var raw struct {
		Data map[string]any `json:"data"`
	}
	b, err := fs.ReadFile(src, name)
	if err != nil {
		return out // файла нет — обычное дело, поиск просто недоступен
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		log.Printf("названия предметов %s: %v", name, err)
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
