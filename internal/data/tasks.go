package data

import (
	"encoding/json"
	"io/fs"
	"log"
	"sort"
	"strings"

	"github.com/Vadimushka/tarkov-map/internal/mapgeo"
)

// Task — задание торговца, у которого есть дело на карте. Trader — имя по-русски, "" — незнакомый.
type Task struct {
	ID, Name, Trader string
	Kappa            bool // нужно для «Каппы»
	Objectives       []Objective
}

// Objective — цель задания. Zones и Points — только на карте, для которой взято задание.
type Objective struct {
	ID, Text string
	Optional bool
	Zones    []Zone        // куда прийти, что отметить
	Points   []mapgeo.Vec3 // где может лежать предмет задания
	Item     string        // название предмета задания
	Keys     []string      // нужные ключи; у каждого — варианты через «или»
}

// Zone — область цели: контур и высоты, по которым определяется этаж.
type Zone struct {
	Pos         mapgeo.Vec3
	Outline     []mapgeo.Vec3
	Top, Bottom float64
}

// Имена торговцев: в переводах заданий их нет, а торговцы меняются редко.
var traders = map[string]string{
	"54cb50c76803fa8b248b4571": "Прапор",
	"54cb57776803fa99248b456e": "Терапевт",
	"579dc571d53a0658a154fbec": "Скупщик",
	"58330581ace78e27b8b10cee": "Лыжник",
	"5935c25fb3acc3127c3d8cd9": "Миротворец",
	"5a7c2eca46aef81a7ca2145d": "Механик",
	"5ac3b934156ae10c4430e83c": "Барахольщик",
	"5c0647fdd443bc2504c2d371": "Егерь",
	"638f541a29ffd1183d187f57": "Смотритель",
	"6617beeaa9cfa777ca915b7c": "Реф",
	"656f0f98d80a697f855d34b1": "Водитель БТР",
}

// --- разбор json.tarkov.dev/{mode}/tasks ---

type rawZone struct {
	Map      string        `json:"map"`
	Position mapgeo.Vec3   `json:"position"`
	Outline  []mapgeo.Vec3 `json:"outline"`
	Top      float64       `json:"top"`
	Bottom   float64       `json:"bottom"`
}

type rawTask struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	NormalizedName string  `json:"normalizedName"`
	Trader         string  `json:"trader"`
	Map            *string `json:"map"`
	KappaRequired  bool    `json:"kappaRequired"`
	Objectives     []struct {
		ID                string    `json:"id"`
		Description       string    `json:"description"`
		Optional          bool      `json:"optional"`
		Maps              []string  `json:"maps"`
		Zones             []rawZone `json:"zones"`
		PossibleLocations []struct {
			Map       string        `json:"map"`
			Positions []mapgeo.Vec3 `json:"positions"`
		} `json:"possibleLocations"`
		QuestItem    string     `json:"questItem"`
		RequiredKeys [][]string `json:"requiredKeys"`
	} `json:"objectives"`
}

// parseTasks читает задания режима и раскладывает их по id карт: задание попадает на карту,
// если оно на ней (map) или там есть его цель. Нет файла — заданий нет, остальное работает.
func parseTasks(src fs.FS, name, ruName string, items map[string]ItemName) (map[string][]Task, int) {
	out := map[string][]Task{}
	b, err := fs.ReadFile(src, name)
	if err != nil {
		return out, 0 // снимок без заданий — квестов просто нет
	}
	var raw struct {
		Data struct {
			Tasks      json.RawMessage `json:"tasks"` // словарь по id или список
			QuestItems map[string]struct {
				Name string `json:"name"`
			} `json:"questItems"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		log.Printf("задания %s: %v", name, err)
		return out, 0
	}
	var list []rawTask
	if err := json.Unmarshal(raw.Data.Tasks, &list); err != nil {
		var byID map[string]rawTask
		if err := json.Unmarshal(raw.Data.Tasks, &byID); err != nil {
			log.Printf("задания %s: %v", name, err)
			return out, 0
		}
		for _, t := range byID {
			list = append(list, t)
		}
	}
	tr := loadTranslations(src, ruName)
	text := func(key, fallback string) string {
		if s := tr(key); s != key {
			return s
		}
		return fallback
	}
	for _, rt := range list {
		maps := map[string]bool{}
		if rt.Map != nil && *rt.Map != "" {
			maps[*rt.Map] = true
		}
		for _, o := range rt.Objectives {
			for _, m := range o.Maps {
				maps[m] = true
			}
			for _, z := range o.Zones {
				maps[z.Map] = true
			}
			for _, l := range o.PossibleLocations {
				maps[l.Map] = true
			}
		}
		for mapID := range maps {
			t := Task{ID: rt.ID, Name: text(rt.Name, rt.NormalizedName), Trader: traders[rt.Trader], Kappa: rt.KappaRequired}
			for _, o := range rt.Objectives {
				obj := Objective{ID: o.ID, Text: text(o.Description, ""), Optional: o.Optional}
				for _, z := range o.Zones {
					if z.Map == mapID {
						obj.Zones = append(obj.Zones, Zone{Pos: z.Position, Outline: z.Outline, Top: z.Top, Bottom: z.Bottom})
					}
				}
				for _, l := range o.PossibleLocations {
					if l.Map == mapID {
						obj.Points = append(obj.Points, l.Positions...)
					}
				}
				if q, ok := raw.Data.QuestItems[o.QuestItem]; ok {
					obj.Item = text(q.Name, "")
				}
				obj.Keys = keyNames(o.RequiredKeys, items)
				t.Objectives = append(t.Objectives, obj)
			}
			out[mapID] = append(out[mapID], t)
		}
	}
	for _, ts := range out {
		sort.Slice(ts, func(i, j int) bool {
			if ts[i].Trader != ts[j].Trader {
				return ts[i].Trader < ts[j].Trader
			}
			return ts[i].Name < ts[j].Name
		})
	}
	return out, len(list)
}

// keyNames: [[id, id], [id]] → «Ключ А или Ключ Б», «Ключ В»; ключи без названия пропускаются.
func keyNames(req [][]string, items map[string]ItemName) []string {
	var out []string
	for _, alts := range req {
		var names []string
		for _, id := range alts {
			if n, ok := items[id]; ok && n.Name != "" {
				names = append(names, n.Name)
			}
		}
		if len(names) > 0 {
			out = append(out, strings.Join(names, " или "))
		}
	}
	return out
}

// Tasks — задания, у которых есть дело на карте с этим id (GameMap.ID).
func (s *Store) Tasks(mapID string) []Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tasks[mapID]
}

// TaskCount — сколько заданий в данных режима (0 — нет файла заданий).
func (s *Store) TaskCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.taskCount
}
