package data

import "testing"

func TestLabels(t *testing.T) {
	s := loaded(t)
	ls := s.Labels("interchange")
	if len(ls) != 3 {
		t.Fatalf("Labels = %+v", ls)
	}
	byText := map[string]Label{}
	for _, l := range ls {
		byText[l.Text] = l
	}
	ps, ok := byText["Электростанция"]
	if !ok || ps.X != -186 || ps.Z != -319 || ps.Bottom == nil || *ps.Bottom != -1 || ps.Top == nil || *ps.Top != 10 {
		t.Errorf("Электростанция: %+v", ps)
	}
	if _, ok := byText["IDEA"]; !ok {
		t.Error("IDEA должна остаться как есть")
	}
	if g, ok := byText["Гараж A"]; !ok || g.Bottom != nil || g.Top != nil {
		t.Errorf("Гараж A: %+v", g)
	}
	if len(s.Labels("factory")) != 0 {
		t.Error("у Завода в фикстуре подписей нет")
	}
}

func TestPlaceName(t *testing.T) {
	cases := map[string]string{"Dorms": "Общаги", "Warehouse 17": "Склад 17", "Pharmacy 2": "Аптека 2", "Kiba": "Kiba", "Garage": "Гараж"}
	for in, want := range cases {
		if got := placeName(in); got != want {
			t.Errorf("placeName(%q) = %q, want %q", in, got, want)
		}
	}
}
