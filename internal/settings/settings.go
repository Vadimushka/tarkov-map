// Package settings хранит настройки приложения в settings.json.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type WindowPos struct {
	X, Y, W, H int
	Maximised  bool
}

type Settings struct {
	GameDir, ShotsDir string          // ручные пути; "" — автопоиск
	Side              string          // "pmc" | "scav"
	Filters           map[string]bool // ключи задаёт фронтенд
	LootQuery         string          // строка поиска лута на земле
	HiddenTasks       []string        // id заданий, снятых в списке заданий
	Presets           []Preset        // наборы слоёв, сохранённые пользователем
	Window            *WindowPos
}

// Preset — набор флажков слоёв под своим названием.
type Preset struct {
	Name    string
	Filters map[string]bool
}

const fileName = "settings.json"

// Load читает настройки; нет файла или он битый — значения по умолчанию.
func Load(dir string) Settings {
	var s Settings
	if b, err := os.ReadFile(filepath.Join(dir, fileName)); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	if s.Side != "scav" {
		s.Side = "pmc"
	}
	if s.Filters == nil {
		s.Filters = map[string]bool{}
	}
	return s
}

func Save(dir string, s Settings) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, fileName+".tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, fileName))
}
