package data

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Vadimushka/tarkov-map/internal/mapgeo"
)

// snapshotAt — время файлов в тестовом снимке.
var snapshotAt = time.Date(2026, 10, 30, 21, 32, 0, 0, time.Local)

// snapshot — снимок из testdata с путями, как их кладёт scripts/seed.
func snapshot(t *testing.T) fstest.MapFS {
	t.Helper()
	m := fstest.MapFS{}
	for name, src := range map[string]string{
		"maps.json":           "testdata/maps.json",
		"pve_maps.json":       "testdata/pve_maps.json",
		"pve_maps_ru.json":    "testdata/pve_maps_ru.json",
		"pve_items_ru.json":   "testdata/pve_items_ru.json",
		"pve_tasks.json":      "testdata/pve_tasks.json",
		"pve_tasks_ru.json":   "testdata/pve_tasks_ru.json",
		"svg/Interchange.svg": "testdata/Interchange.svg",
	} {
		b, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		m[name] = &fstest.MapFile{Data: b, ModTime: snapshotAt}
	}
	return m
}

func loaded(t *testing.T) *Store {
	t.Helper()
	s := NewStore(snapshot(t))
	if err := s.Load("pve"); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestLoadAndLookup(t *testing.T) {
	s := loaded(t)
	m, ok := s.MapByScene("maps/shopping_mall.bundle")
	if !ok {
		t.Fatal("Развязка не найдена по scenePath")
	}
	if m.Name != "Развязка" || m.NormalizedName != "interchange" {
		t.Errorf("Name/NormalizedName = %q %q", m.Name, m.NormalizedName)
	}
	byName := map[string]Extract{}
	for _, e := range m.Extracts {
		byName[e.Name] = e
	}
	if byName["Безопасная комната"].Faction != "pmc" {
		t.Errorf("Безопасная комната: %+v", byName["Безопасная комната"])
	}
	if byName["Выход на железной дороге"].Faction != "scav" {
		t.Errorf("Выход на железной дороге: %+v", byName["Выход на железной дороге"])
	}
	if !byName["А-Выход у ТЭЦ"].Paid {
		t.Errorf("А-Выход у ТЭЦ не платный: %+v", byName["А-Выход у ТЭЦ"])
	}
	if len(m.Transits) != 1 || m.Transits[0].Name != "Переход на Таможню" {
		t.Errorf("Transits = %+v", m.Transits)
	}
}

// У вариантов карт (Эпицентр 21+, ночной Завод) в снимке у выходов нет faction — без стороны
// они не показывались ни своими, ни совместными. Сторона берётся у того же выхода основной
// карты: по имени и ближайшей точке, потому что одноимённые выходы бывают разных сторон.
func TestVariantExtractFaction(t *testing.T) {
	src := fstest.MapFS{"m.json": {Data: []byte(`{"data":{"maps":{
	  "1":{"normalizedName":"ground-zero","scenePath":"maps/sandbox_preset.bundle","extracts":[
	    {"name":"Sandbox_VExit","faction":"pmc","position":{"x":-19.5,"y":0,"z":114.9}},
	    {"name":"Gate 3","faction":"scav","position":{"x":58.7,"y":0,"z":60.9}},
	    {"name":"Gate 3","faction":"pmc","position":{"x":58.4,"y":0,"z":63.3}}]},
	  "2":{"normalizedName":"ground-zero-21","scenePath":"maps/sandbox_high_preset.bundle","extracts":[
	    {"name":"Sandbox_VExit","position":{"x":-19.5,"y":0,"z":114.9}},
	    {"name":"Gate 3","position":{"x":58.4,"y":0,"z":63.3}},
	    {"name":"Gate 3","position":{"x":58.7,"y":0,"z":60.9}},
	    {"name":"New_exit","position":{"x":0,"y":0,"z":0}}]}}}}`)}}
	parsed, err := parseMaps(src, "m.json", "m_ru.json")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range parsed["maps/sandbox_high_preset.bundle"].Extracts {
		got[fmt.Sprintf("%s@%.0f", e.Name, e.Pos.Z)] = e.Faction
	}
	// незнакомый выход — совместный: лучше показать обеим сторонам, чем спрятать
	want := map[string]string{"Sandbox_VExit@115": "pmc", "Gate 3@63": "pmc", "Gate 3@61": "scav", "New_exit@0": "shared"}
	if !maps.Equal(got, want) {
		t.Errorf("стороны выходов = %v, want %v", got, want)
	}
	if f := parsed["maps/sandbox_preset.bundle"].Extracts[1].Faction; f != "scav" {
		t.Errorf("у основной карты сторона изменилась: %q", f)
	}
}

// В реальных данных switch бывает bool, а бывает строкой — id рубильника (Таможня).
func TestSwitchFlag(t *testing.T) {
	cases := map[string]bool{`false`: false, `true`: true, `"5c1d0dc586f7744baf2e7b79"`: true, `""`: false, `null`: false, ``: false}
	for raw, want := range cases {
		if got := switchFlag([]byte(raw)); got != want {
			t.Errorf("switchFlag(%s) = %v, want %v", raw, got, want)
		}
	}
}

func TestMapDef(t *testing.T) {
	s := loaded(t)
	d, ok := s.MapDef("interchange")
	if !ok {
		t.Fatal("нет MapDef Развязки")
	}
	if d.CoordinateRotation != 180 || d.Layers[0].SVGLayer != "First_Floor" {
		t.Errorf("rotation/layer = %v %q", d.CoordinateRotation, d.Layers[0].SVGLayer)
	}
	if b := d.Layers[0].Extents[0].Bounds[0]; b != [2][2]float64{{120, 218}, {-222, -327}} {
		t.Errorf("bounds = %v", b)
	}
	if f, ok := s.MapDef("night-factory"); !ok || f.Key != "factory" {
		t.Errorf("night-factory → %q %v", f.Key, ok)
	}
	if _, ok := s.MapDef("the-lab"); ok {
		t.Error("the-lab не должна поддерживаться")
	}
}

// Режима нет в снимке или снимка нет вовсе — ErrNoData, а не паника.
func TestLoadMissingIsNoData(t *testing.T) {
	if err := NewStore(snapshot(t)).Load("regular"); !errors.Is(err, ErrNoData) {
		t.Errorf("режима нет в снимке: %v, want ErrNoData", err)
	}
	if err := NewStore(nil).Load("pve"); !errors.Is(err, ErrNoData) {
		t.Errorf("без снимка: %v, want ErrNoData", err)
	}
	if _, err := NewStore(nil).SVG(mapgeo.MapDef{SVGPath: "https://assets.tarkov.dev/maps/svg/Interchange.svg"}); err == nil {
		t.Error("SVG без снимка вернул nil")
	}
}

// Тот же режим повторно не разбирается: Load зовётся на каждую загрузку карты игрой.
func TestLoadSameModeKeepsData(t *testing.T) {
	src := snapshot(t)
	s := NewStore(src)
	if err := s.Load("pve"); err != nil {
		t.Fatal(err)
	}
	src["pve_maps.json"] = &fstest.MapFile{Data: []byte("{битый")}
	if err := s.Load("pve"); err != nil {
		t.Errorf("повторный Load того же режима разбирал файлы заново: %v", err)
	}
	if _, ok := s.MapByScene("maps/shopping_mall.bundle"); !ok {
		t.Error("данные пропали")
	}
}

func TestSVGFromSnapshot(t *testing.T) {
	s := loaded(t)
	want, _ := os.ReadFile("testdata/Interchange.svg")
	d, _ := s.MapDef("interchange")
	if svg, err := s.SVG(d); err != nil || svg != string(want) {
		t.Errorf("SVG Развязки: %d байт, %v", len(svg), err)
	}
	f, _ := s.MapDef("factory")
	if _, err := s.SVG(f); err == nil {
		t.Error("SVG Завода в снимке нет, а ошибки нет")
	}
}

// «Данные от …» в статусе — время снимка.
func TestUpdatedIsSnapshotTime(t *testing.T) {
	if got := loaded(t).Updated(); !got.Equal(snapshotAt) {
		t.Errorf("Updated() = %v, want %v", got, snapshotAt)
	}
}

func TestSVGName(t *testing.T) {
	if got := SVGName(mapgeo.MapDef{SVGPath: "https://assets.tarkov.dev/maps/svg/Interchange.svg"}); got != "svg/Interchange.svg" {
		t.Errorf("SVGName = %q", got)
	}
}

// Список для ручного выбора: только карты, которые можно показать (есть SVG).
func TestMapsListsOnlySupported(t *testing.T) {
	s := loaded(t)
	got := s.Maps()
	want := []MapRef{{Scene: "maps/shopping_mall.bundle", ID: "interchange", Name: "Развязка"}}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("Maps() = %+v, want %+v — у Лаборатории нет SVG", got, want)
	}
}

func TestSVGDefs(t *testing.T) {
	s := loaded(t)
	defs := s.SVGDefs()
	if len(defs) != 2 || path.Base(defs[0].SVGPath) != "Factory.svg" || path.Base(defs[1].SVGPath) != "Interchange.svg" {
		t.Errorf("SVGDefs = %+v, want Factory.svg и Interchange.svg", defs)
	}
}

func TestItemCount(t *testing.T) {
	if n := loaded(t).ItemCount(); n == 0 {
		t.Error("названия предметов из снимка не посчитаны")
	}
	src := snapshot(t)
	delete(src, "pve_items_ru.json")
	s := NewStore(src)
	if err := s.Load("pve"); err != nil {
		t.Fatal(err)
	}
	if n := s.ItemCount(); n != 0 {
		t.Errorf("без файла названий ItemCount = %d", n)
	}
}
