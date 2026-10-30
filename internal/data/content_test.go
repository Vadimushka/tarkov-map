package data

import (
	"bytes"
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
