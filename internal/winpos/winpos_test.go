package winpos

import (
	"testing"
	"unsafe"

	"github.com/Vadimushka/tarkov-map/internal/settings"
)

func TestPlacementSize(t *testing.T) {
	if n := unsafe.Sizeof(placement{}); n != 44 {
		t.Errorf("sizeof(WINDOWPLACEMENT) = %d, want 44", n)
	}
}

func TestFromPlacement(t *testing.T) {
	norm := rect{Left: -1800, Top: 60, Right: -520, Bottom: 860} // второй монитор слева
	cases := []struct {
		name string
		p    placement
		want settings.WindowPos
	}{
		{"обычное", placement{ShowCmd: swShowNormal, Normal: norm}, settings.WindowPos{X: -1800, Y: 60, W: 1280, H: 800}},
		{"развёрнуто", placement{ShowCmd: swShowMaximized, Normal: norm}, settings.WindowPos{X: -1800, Y: 60, W: 1280, H: 800, Maximised: true}},
		{"свёрнуто из развёрнутого", placement{ShowCmd: swShowMinimized, Flags: wpfRestoreToMaximized, Normal: norm}, settings.WindowPos{X: -1800, Y: 60, W: 1280, H: 800, Maximised: true}},
		{"свёрнуто из обычного", placement{ShowCmd: swShowMinimized, Normal: norm}, settings.WindowPos{X: -1800, Y: 60, W: 1280, H: 800}},
	}
	for _, c := range cases {
		if got := fromPlacement(c.p); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestToPlacement(t *testing.T) {
	w := settings.WindowPos{X: -1800, Y: 60, W: 1280, H: 800}
	p := toPlacement(w)
	if p.Length != 44 || p.ShowCmd != swShowNormal || p.Normal != (rect{-1800, 60, -520, 860}) {
		t.Errorf("toPlacement(%+v) = %+v", w, p)
	}
	w.Maximised = true
	if p := toPlacement(w); p.ShowCmd != swShowMaximized || fromPlacement(p) != w {
		t.Errorf("развёрнутое: %+v", p)
	}
}

func TestValid(t *testing.T) {
	for _, w := range []settings.WindowPos{{W: 0, H: 800}, {W: 1280, H: -1}, {W: 50, H: 50}} {
		if valid(w) {
			t.Errorf("valid(%+v) = true", w)
		}
	}
	if !valid(settings.WindowPos{X: -1800, Y: 60, W: 1280, H: 800}) {
		t.Error("обычное окно не прошло")
	}
}
