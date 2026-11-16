package data

import (
	"maps"
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

// Логи хранят немного сессий: что сдано до самой старой, в них не записано. Но взятое или
// закрытое задание говорит о своих требованиях: «сдано» — сдано, «провалено» — провалено,
// «активно» — взято (статуса нет, но сдано всё, что требовалось для него), и так по цепочке.
func TestInferQuestStatus(t *testing.T) {
	req := map[string][]TaskReq{
		"c": {{ID: "b", Status: "done"}},
		"b": {{ID: "a", Status: "done"}, {ID: "x", Status: "taken"}},
		"x": {{ID: "y", Status: "done"}},
		"e": {{ID: "f", Status: "fail"}},
		"g": {{ID: "h", Status: "done"}},
		"z": {{ID: "c", Status: "done"}}, // z без записи — о c ничего не говорит
	}
	known := map[string]string{"c": "start", "e": "done", "h": "fail", "g": "start", "a": "start"}
	got := inferQuestStatus(known, req)
	// a: запись «взято» устарела — его требует сданным взятое b; h: провал из логов не перекрывается
	want := map[string]string{"c": "start", "b": "done", "a": "done", "y": "done", "e": "done", "f": "fail", "g": "start", "h": "fail"}
	if !maps.Equal(got, want) {
		t.Errorf("статусы = %v, want %v", got, want)
	}
	if len(known) != 5 {
		t.Errorf("исходные статусы изменены: %v", known)
	}
}

// Требования берутся из снимка: t_lab требует сданным t_ic, t_ic — «активным или сданным»
// t_hideout, а тот — сданным t_untranslated.
func TestStoreInferQuestStatus(t *testing.T) {
	got := loaded(t).InferQuestStatus(map[string]string{"t_lab": "start"})
	want := map[string]string{"t_lab": "start", "t_ic": "done", "t_untranslated": "done"}
	if !maps.Equal(got, want) {
		t.Errorf("статусы = %v, want %v", got, want)
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
