package settings

import (
	"reflect"
	"testing"
)

func TestSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := Settings{
		GameDir: `C:\SteamLibrary\steamapps\common\Escape from Tarkov\build`,
		Side:    "scav",
		Filters: map[string]bool{"route": true, "extract_other": false},
		Window:  &WindowPos{X: 1920, Y: 0, W: 1280, H: 800},
	}
	if err := Save(dir, in); err != nil {
		t.Fatal(err)
	}
	if out := Load(dir); !reflect.DeepEqual(in, out) {
		t.Errorf("Load = %+v, want %+v", out, in)
	}
}

func TestSettingsDefaults(t *testing.T) {
	s := Load(t.TempDir())
	if s.Side != "pmc" || s.Filters == nil || s.Window != nil {
		t.Errorf("по умолчанию: %+v", s)
	}
}
