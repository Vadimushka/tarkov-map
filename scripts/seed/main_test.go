package main

import (
	"archive/zip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Vadimushka/tarkov-map/internal/data"
)

// fakeTarkovDev — сервер с файлами internal/data/testdata по путям tarkov.dev. bodies
// подменяет ответ по пути, пустая строка — 404. SVG по основному адресу нет — берётся запасной.
func fakeTarkovDev(t *testing.T, bodies map[string]string) *downloader {
	t.Helper()
	td := "../../internal/data/testdata/"
	files := map[string]string{
		"/maps.json":          "maps.json",
		"/fb/Interchange.svg": "Interchange.svg",
		"/fb/Factory.svg":     "Interchange.svg",
		"/pve/maps":           "pve_maps.json",
		"/pve/maps_ru":        "pve_maps_ru.json",
		"/pve/items_ru":       "pve_items_ru.json",
		"/pve/tasks":          "pve_tasks.json",
		"/pve/tasks_ru":       "pve_tasks_ru.json",
		"/regular/maps":       "pve_maps.json",
		"/regular/maps_ru":    "pve_maps_ru.json",
		"/regular/items_ru":   "pve_items_ru.json",
		"/regular/tasks":      "pve_tasks.json",
		"/regular/tasks_ru":   "pve_tasks_ru.json",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if b, ok := bodies[r.URL.Path]; ok && b != "" {
			w.Write([]byte(b))
			return
		} else if ok {
			http.NotFound(w, r)
			return
		}
		f, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, td+f)
	}))
	t.Cleanup(srv.Close)
	return &downloader{
		jsonBase: srv.URL, mapsJSON: srv.URL + "/maps.json",
		svgBase: srv.URL + "/svg", svgFallback: srv.URL + "/fb", client: srv.Client(),
	}
}

func TestRunBuildsSnapshot(t *testing.T) {
	out := filepath.Join(t.TempDir(), "data.zip")
	if err := run(out, fakeTarkovDev(t, nil)); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, mode := range []string{"pve", "regular"} {
		s := data.NewStore(zr)
		if err := s.Load(mode); err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		if len(s.Tasks("5714dbc024597771384a510d")) == 0 {
			t.Errorf("%s: нет заданий", mode)
		}
		if _, ok := s.ItemName("i_led"); !ok {
			t.Errorf("%s: нет названий предметов", mode)
		}
		d, _ := s.MapDef("interchange")
		if svg, err := s.SVG(d); err != nil || svg == "" {
			t.Errorf("%s: SVG Развязки (с запасного адреса): %v", mode, err)
		}
	}
}

// Без названий предметов у друга не заработает поиск лута: снимок не собирается,
// прежний остаётся.
func TestRunKeepsOldSnapshotOnFailure(t *testing.T) {
	out := filepath.Join(t.TempDir(), "data.zip")
	os.WriteFile(out, []byte("прежний"), 0o644)
	if err := run(out, fakeTarkovDev(t, map[string]string{"/regular/items_ru": ""})); err == nil {
		t.Error("снимок без названий предметов собрался")
	}
	if b, _ := os.ReadFile(out); string(b) != "прежний" {
		t.Error("прежний снимок испорчен")
	}
}

// Сервер ответил 200, но не тем: снимок с битыми названиями или «картой»-страницей
// ошибки ушёл бы другу молча — обновить его в программе уже нечем.
func TestRunRejectsBrokenFiles(t *testing.T) {
	for name, bodies := range map[string]map[string]string{
		"битые названия":  {"/pve/items_ru": `{"data": {"i_led Name": "LEDX"`},
		"пустые названия": {"/regular/items_ru": `{"errors": ["rate limit"]}`},
		"SVG — не SVG":    {"/fb/Factory.svg": "<html>502 Bad Gateway</html>"},
		"пустые задания":  {"/pve/tasks": `{"errors": ["rate limit"]}`},
		"нет заданий":     {"/regular/tasks": ""},
	} {
		t.Run(name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "data.zip")
			os.WriteFile(out, []byte("прежний"), 0o644)
			if err := run(out, fakeTarkovDev(t, bodies)); err == nil {
				t.Error("снимок собрался")
			}
			if b, _ := os.ReadFile(out); string(b) != "прежний" {
				t.Error("прежний снимок испорчен")
			}
		})
	}
}
