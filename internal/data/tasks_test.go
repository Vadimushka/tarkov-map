package data

import (
	"strings"
	"testing"
)

const (
	interchangeID = "5714dbc024597771384a510d"
	labID         = "5b0fc42d86f7744a585f9105"
)

func TestTasksOfMap(t *testing.T) {
	s := loaded(t)
	got := s.Tasks(interchangeID)
	// по торговцу, затем по названию; без перевода — normalizedName, неизвестный торговец — ""
	if len(got) != 2 || got[0].Name != "only-map" || got[0].Trader != "" || got[1].ID != "t_ic" {
		t.Fatalf("задания Развязки = %+v", got)
	}
	ic := got[1]
	if ic.Name != "Задание в ТЦ" || ic.Trader != "Прапор" || !ic.Kappa || len(ic.Objectives) != 3 {
		t.Errorf("задание = %+v", ic)
	}
	visit := ic.Objectives[0]
	if visit.ID != "o_visit" || visit.Text != "Посетить ТЦ" || visit.Optional || len(visit.Zones) != 1 || len(visit.Points) != 0 {
		t.Errorf("visit = %+v", visit)
	} else if z := visit.Zones[0]; len(z.Outline) != 3 || z.Top != 33 || z.Bottom != 27 || z.Pos.X != 10 {
		t.Errorf("зона другой карты не отброшена или разобрана не так: %+v", z)
	}
	find := ic.Objectives[1]
	// ключ без названия в данных не показывается
	if !find.Optional || find.Item != "Флешка с данными" || len(find.Points) != 2 || strings.Join(find.Keys, ";") != "Ключ от КПП" {
		t.Errorf("find = %+v", find)
	}
	if shoot := ic.Objectives[2]; shoot.Text != "Ликвидировать Диких" || len(shoot.Zones)+len(shoot.Points) != 0 {
		t.Errorf("shoot = %+v", shoot)
	}

	lab := s.Tasks(labID)
	if len(lab) != 2 || lab[0].ID != "t_lab" || lab[0].Trader != "Егерь" || lab[1].ID != "t_ic" {
		t.Fatalf("задания Лаборатории = %+v", lab)
	}
	if z := lab[1].Objectives[0].Zones; len(z) != 1 || z[0].Pos.X != 1 {
		t.Errorf("зоны на Лаборатории = %+v", z)
	}
	if n := len(s.Tasks("нет-такой")); n != 0 {
		t.Errorf("заданий на неизвестной карте: %d", n)
	}
	if s.TaskCount() != 4 {
		t.Errorf("TaskCount = %d", s.TaskCount())
	}
}

// Файлы заданий необязательны: без них нет только квестов.
func TestTasksOptional(t *testing.T) {
	fs := snapshot(t)
	delete(fs, "pve_tasks.json")
	delete(fs, "pve_tasks_ru.json")
	s := NewStore(fs)
	if err := s.Load("pve"); err != nil {
		t.Fatal(err)
	}
	if len(s.Tasks(interchangeID)) != 0 || s.TaskCount() != 0 {
		t.Error("задания без файлов")
	}
	if _, ok := s.MapByScene("maps/shopping_mall.bundle"); !ok {
		t.Error("без заданий пропали карты")
	}
}
