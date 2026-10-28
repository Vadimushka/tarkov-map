// Package shots разбирает имена скриншотов EFT и следит за папкой скриншотов.
package shots

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/Vadimushka/tarkov-map/internal/mapgeo"
)

// Shot — позиция и поворот камеры, записанные игрой в имя скриншота.
type Shot struct {
	Name       string
	Date, Time string // "2026-10-28", "21:06"
	Pos        mapgeo.Vec3
	Q          [4]float64 // qx, qy, qz, qw
}

// 2026-10-28[21-06]_x, y, z_qx, qy, qz, qw_<игровое время> (n).png
var nameRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\[(\d{2})-(\d{2})\]_(-?[\d.]+), (-?[\d.]+), (-?[\d.]+)_(-?[\d.]+), (-?[\d.]+), (-?[\d.]+), (-?[\d.]+)_.*\.png$`)

// ParseName разбирает имя файла скриншота.
func ParseName(name string) (Shot, bool) {
	m := nameRe.FindStringSubmatch(name)
	if m == nil {
		return Shot{}, false
	}
	var v [7]float64
	for i := range v {
		f, err := strconv.ParseFloat(m[4+i], 64)
		if err != nil {
			return Shot{}, false
		}
		v[i] = f
	}
	return Shot{
		Name: name,
		Date: m[1],
		Time: m[2] + ":" + m[3],
		Pos:  mapgeo.Vec3{X: v[0], Y: v[1], Z: v[2]},
		Q:    [4]float64{v[3], v[4], v[5], v[6]},
	}, true
}

// Key одинаков для скринов с одной позицией и поворотом ("(0)", "(1)" одной секунды).
func (s Shot) Key() string {
	return fmt.Sprintf("%.2f|%.2f|%.2f|%.5f", s.Pos.X, s.Pos.Y, s.Pos.Z, s.Q[1])
}
