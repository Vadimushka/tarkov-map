// Package winpos запоминает и восстанавливает положение окна через Win32 WINDOWPLACEMENT:
// координаты абсолютные, поэтому окно возвращается на тот монитор, где было.
package winpos

import "github.com/Vadimushka/tarkov-map/internal/settings"

type point struct{ X, Y int32 }

type rect struct{ Left, Top, Right, Bottom int32 }

// placement повторяет раскладку WINDOWPLACEMENT.
type placement struct {
	Length  uint32
	Flags   uint32
	ShowCmd uint32
	Min     point
	Max     point
	Normal  rect // положение в обычном (не развёрнутом) состоянии
}

const (
	swShowNormal          = 1
	swShowMinimized       = 2
	swShowMaximized       = 3
	wpfRestoreToMaximized = 2
	placementSize         = 44
	minSide               = 200 // меньше — считаем мусором в настройках
)

func fromPlacement(p placement) settings.WindowPos {
	return settings.WindowPos{
		X: int(p.Normal.Left), Y: int(p.Normal.Top),
		W: int(p.Normal.Right - p.Normal.Left), H: int(p.Normal.Bottom - p.Normal.Top),
		Maximised: p.ShowCmd == swShowMaximized ||
			(p.ShowCmd == swShowMinimized && p.Flags&wpfRestoreToMaximized != 0),
	}
}

func toPlacement(w settings.WindowPos) placement {
	p := placement{
		Length:  placementSize,
		ShowCmd: swShowNormal,
		Normal:  rect{int32(w.X), int32(w.Y), int32(w.X + w.W), int32(w.Y + w.H)},
	}
	if w.Maximised {
		p.ShowCmd = swShowMaximized
	}
	return p
}

func valid(w settings.WindowPos) bool {
	return w.W >= minSide && w.H >= minSide
}
