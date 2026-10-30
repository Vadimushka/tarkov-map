// Package mapgeo пересчитывает игровые координаты в координаты SVG-карты
// так же, как это делает tarkov.dev (Leaflet CRS с transform и поворотом).
package mapgeo

import "math"

// Vec3 — игровые координаты; Y — высота.
type Vec3 struct{ X, Y, Z float64 }

// Extent — область этажа: диапазон высот [lo, hi) и прямоугольники в порядке [x, z].
type Extent struct {
	Height [2]float64
	Bounds [][2][2]float64
}

// Layer — этаж карты; SVGLayer — id верхнеуровневой группы в SVG.
type Layer struct {
	Name, SVGLayer string
	Extents        []Extent
}

// MapDef — параметры карты из maps.json tarkov.dev.
type MapDef struct {
	Key                string
	Transform          []float64 // [sx, mx, sy, my]; пусто = [1, 0, 1, 0]
	CoordinateRotation float64   // градусы
	Bounds             [2][2]float64
	SVGBounds          *[2][2]float64
	SVGPath, SVGLayer  string
	Layers             []Layer
}

// Rect — прямоугольник в координатах карты.
type Rect struct{ X, Y, W, H float64 }

// Project переводит игровые (x, z) в координаты карты.
func Project(md MapDef, x, z float64) (px, py float64) {
	r := md.CoordinateRotation * math.Pi / 180
	c, s := math.Cos(r), math.Sin(r)
	rx := x*c - z*s
	ry := x*s + z*c
	sx, mx, sy, my := 1.0, 0.0, 1.0, 0.0
	if len(md.Transform) == 4 {
		sx, mx, sy, my = md.Transform[0], md.Transform[1], md.Transform[2], md.Transform[3]
	}
	return sx*rx + mx, -sy*ry + my
}

// ImageRect — прямоугольник, на который растянута SVG-карта.
func ImageRect(md MapDef) Rect {
	b := md.Bounds
	if md.SVGBounds != nil {
		b = *md.SVGBounds
	}
	minX, maxX := math.Min(b[0][0], b[1][0]), math.Max(b[0][0], b[1][0])
	minZ, maxZ := math.Min(b[0][1], b[1][1]), math.Max(b[0][1], b[1][1])
	x1, y1 := Project(md, minX, maxZ)
	x2, y2 := Project(md, maxX, minZ)
	return Rect{
		X: math.Min(x1, x2),
		Y: math.Min(y1, y2),
		W: math.Abs(x2 - x1),
		H: math.Abs(y2 - y1),
	}
}

// LayerOf возвращает SVG-слой этажа, на котором находится точка.
func LayerOf(md MapDef, p Vec3) string {
	for _, l := range md.Layers {
		for _, e := range l.Extents {
			if p.Y < e.Height[0] || p.Y >= e.Height[1] {
				continue
			}
			if len(e.Bounds) == 0 {
				return l.SVGLayer // без прямоугольников этаж действует на всю карту
			}
			for _, bb := range e.Bounds {
				if inside(bb, p.X, p.Z) {
					return l.SVGLayer
				}
			}
		}
	}
	return md.SVGLayer
}

// LayerOfSpan — этаж подписи с диапазоном высот [bottom, top]: тот, в один extent которого
// диапазон укладывается целиком (границы включительно) и в чьи прямоугольники попадает
// точка. Диапазон шире любого этажа (санаторий Берега: -100…100) — базовый слой.
func LayerOfSpan(md MapDef, x, z, bottom, top float64) string {
	for _, l := range md.Layers {
		for _, e := range l.Extents {
			if bottom < e.Height[0] || top > e.Height[1] {
				continue
			}
			if len(e.Bounds) == 0 {
				return l.SVGLayer
			}
			for _, bb := range e.Bounds {
				if inside(bb, x, z) {
					return l.SVGLayer
				}
			}
		}
	}
	return md.SVGLayer
}

func inside(bb [2][2]float64, x, z float64) bool {
	return x >= math.Min(bb[0][0], bb[1][0]) && x <= math.Max(bb[0][0], bb[1][0]) &&
		z >= math.Min(bb[0][1], bb[1][1]) && z <= math.Max(bb[0][1], bb[1][1])
}

// Forward — направление взгляда в плоскости (x, z) по кватерниону камеры Unity.
func Forward(qx, qy, qz, qw float64) (fx, fz float64) {
	return 2 * (qx*qz + qw*qy), 1 - 2*(qx*qx+qy*qy)
}
