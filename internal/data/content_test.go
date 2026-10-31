package data

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Vadimushka/tarkov-map/internal/mapgeo"
)

func TestLoadMapContent(t *testing.T) {
	s := loaded(t)
	m, _ := s.MapByScene("maps/shopping_mall.bundle")
	if len(m.Containers) != 2 || m.Containers[0] != (Container{Group: "weapon", Type: "weapon-box", Name: "Оружейный кофр", Pos: mapgeo.Vec3{X: 10, Y: 1, Z: 20}}) {
		t.Fatalf("Containers = %+v", m.Containers)
	}
	if m.Containers[1].Group != "misc" || m.Containers[1].Name != "Таинственный ящик" {
		t.Errorf("незнакомый тип: %+v", m.Containers[1])
	}
	if len(m.Loose) != 1 || strings.Join(m.Loose[0].Items, ",") != "i_flash,i_led" {
		t.Errorf("Loose = %+v", m.Loose)
	}
	if len(m.Locks) != 1 || m.Locks[0].Kind != "door" || m.Locks[0].KeyID != "i_key" || !m.Locks[0].NeedsPower {
		t.Errorf("Locks = %+v", m.Locks)
	}
	if len(m.Switches) != 1 || m.Switches[0].Name != "Главный выключатель питания ТЦ" {
		t.Errorf("Switches = %+v", m.Switches)
	}
	if len(m.Hazards) != 1 || m.Hazards[0].Kind != "minefield" || len(m.Hazards[0].Outline) != 3 {
		t.Errorf("Hazards = %+v", m.Hazards)
	}
	if len(m.Weapons) != 1 || !strings.Contains(m.Weapons[0].Name, "Утес") {
		t.Errorf("Weapons = %+v", m.Weapons)
	}
	if len(m.Bosses) != 1 || m.Bosses[0].Name != "Килла" || m.Bosses[0].Chance != 0.75 ||
		len(m.Bosses[0].Spots) != 2 || m.Bosses[0].Spots[0].Chance != 0.33 {
		t.Errorf("Bosses = %+v", m.Bosses)
	}
}

func TestContainerGroup(t *testing.T) {
	cases := map[string]string{
		"weapon-box": "weapon", "grenade-box": "weapon", "wooden-ammo-box": "weapon",
		"medbag": "med", "medcase": "med", "medical-supply-crate": "med",
		"pc-block": "tech", "toolbox": "tech", "technical-supply-crate": "tech",
		"safe": "safe", "bank-safe": "safe", "cash-register": "safe", "bank-cash-register": "safe",
		"buried-barrel-cache": "cache", "ground-cache": "cache", "shturmans-stash": "cache",
		"dead-scav": "body", "scav-body": "body", "pmc-body": "body", "civilian-body": "body", "lab-technician-body": "body",
		"duffle-bag": "misc", "drawer": "misc", "jacket": "misc", "something-new": "misc",
	}
	for n, want := range cases {
		if got := containerGroup(n); got != want {
			t.Errorf("containerGroup(%q) = %q, want %q", n, got, want)
		}
	}
}

func TestItemNames(t *testing.T) {
	s := loaded(t)
	if n, ok := s.ItemName("i_led"); !ok || n != (ItemName{Name: "Транспортир LEDX", Short: "LEDX"}) {
		t.Errorf("i_led = %+v %v", n, ok)
	}
	if _, ok := s.ItemName("MOD_STOCK"); ok {
		t.Error("ключи без суффикса Name/ShortName — не предметы")
	}
	if _, ok := s.ItemName("nope"); ok {
		t.Error("нет такого предмета")
	}
}

func TestMapsWorkWithoutItemsFile(t *testing.T) {
	src := snapshot(t)
	delete(src, "pve_items_ru.json")
	s := NewStore(src)
	if err := s.Load("pve"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.MapByScene("maps/shopping_mall.bundle"); !ok {
		t.Error("карта должна загрузиться без items_ru")
	}
	if _, ok := s.ItemName("i_led"); ok {
		t.Error("названий быть не должно")
	}
}

// Битый файл названий не ломает карту, но причина «нет названий» видна в логе.
func TestBrokenItemsLogged(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	src := fstest.MapFS{"pve_items_ru.json": {Data: []byte(`{"data": {"i_flash Name": "Флешка"`)}}
	if n := len(parseItems(src, "pve_items_ru.json")); n != 0 {
		t.Errorf("из битого файла разобрано %d названий", n)
	}
	if !strings.Contains(buf.String(), "pve_items_ru.json") {
		t.Errorf("битый файл названий не попал в лог: %q", buf.String())
	}
}

func TestLoadSpawnsBTRAndLinks(t *testing.T) {
	s := loaded(t)
	m, _ := s.MapByScene("maps/shopping_mall.bundle")
	kinds := []string{}
	for _, sp := range m.Spawns {
		kinds = append(kinds, sp.Kind)
	}
	// «all» — и ЧВК, и Дикие; спавн босса не дублирует слой боссов
	if strings.Join(kinds, ",") != "pmc,scav,sniper,pmc,scav,sniper,sniper" {
		t.Errorf("Spawns kinds = %v", kinds)
	}
	if len(m.BTRStops) != 1 || m.BTRStops[0].Name != "КПП USEC" || m.BTRStops[0].Pos != (mapgeo.Vec3{X: 7, Y: 1, Z: 8}) {
		t.Errorf("BTRStops = %+v", m.BTRStops)
	}
	if m.RaidMinutes != 40 {
		t.Errorf("RaidMinutes = %d", m.RaidMinutes)
	}
	if len(m.Switches) != 1 || m.Switches[0].ID != "sw_mall" || strings.Join(m.Switches[0].Activates, ",") != "sw_door" {
		t.Errorf("Switches = %+v", m.Switches)
	}
	var auto Extract
	for _, e := range m.Extracts {
		if e.Name == "А-Выход у ТЭЦ" {
			auto = e
		}
	}
	if len(auto.Outline) != 3 || auto.Item != "5449016a4bdc2d6f028b456f" || auto.ItemCount != 20000 ||
		strings.Join(auto.Switches, ",") != "sw_mall" {
		t.Errorf("А-Выход у ТЭЦ = %+v", auto)
	}
}

// На Таможне в снимке у всех 28 выходов один и тот же рубильник ZB-013 — ошибка данных:
// рубильник, указанный у каждого выхода карты (от 5 выходов), не считается условием выхода.
func TestSwitchOnEveryExtractIgnored(t *testing.T) {
	s := loaded(t)
	m, _ := s.MapByScene("maps/laboratory_preset.bundle")
	got := []string{}
	for _, e := range m.Extracts {
		got = append(got, fmt.Sprintf("%s:%v:%s", e.Name, e.Switch, strings.Join(e.Switches, "/")))
	}
	if want := "l1:false:,l2:false:,l3:false:,l4:false:,l5:true:sw_gate"; strings.Join(got, ",") != want {
		t.Errorf("выходы = %s, want %s", strings.Join(got, ","), want)
	}
}

// Снайпер — по зоне «snip…» или по категории sniper без player: в PvE-снимке Эпицентра у сотни
// точек игроков categories ["player","sniper"], и слой «Снайперы» засыпало ромбами.
func TestSpawnKinds(t *testing.T) {
	for _, c := range []struct {
		sides, cats []string
		zone, want  string
	}{
		{[]string{"all"}, []string{"player", "sniper"}, "8f2c1d3e-uuid", "[pmc scav]"},
		{[]string{"scav"}, []string{"bot", "sniper"}, "ZoneRoad", "[sniper]"},
		{[]string{"scav"}, []string{"bot"}, "ZoneSnipeBrige", "[sniper]"},
		{[]string{"all"}, []string{"player"}, "ZoneSniper", "[pmc sniper]"},
		{[]string{"pmc"}, []string{"botpmc"}, "ZoneDormitory", "[pmc]"},
		{[]string{"all"}, []string{"boss"}, "ZoneBrige", "[]"},
	} {
		if got := fmt.Sprint(spawnKinds(c.sides, c.cats, c.zone)); got != c.want {
			t.Errorf("spawnKinds(%v, %v, %q) = %s, want %s", c.sides, c.cats, c.zone, got, c.want)
		}
	}
}
