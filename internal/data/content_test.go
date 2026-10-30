package data

import (
	"strings"
	"testing"

	"github.com/Vadimushka/tarkov-map/internal/mapgeo"
)

func TestLoadMapContent(t *testing.T) {
	s, _, _, _ := loaded(t)
	m, _ := s.MapByScene("maps/shopping_mall.bundle")
	if len(m.Containers) != 2 || m.Containers[0] != (Container{Group: "weapon", Name: "Оружейный кофр", Pos: mapgeo.Vec3{X: 10, Y: 1, Z: 20}}) {
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
