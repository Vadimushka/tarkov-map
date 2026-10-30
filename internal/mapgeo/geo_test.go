package mapgeo

import (
	"math"
	"testing"
)

// Развязка — значения из maps.json tarkov.dev.
var ic = MapDef{
	Key:                "interchange",
	Transform:          []float64{0.265, 150.6, 0.265, 134.6},
	CoordinateRotation: 180,
	Bounds:             [2][2]float64{{598, -442}, {-433, 426}},
	SVGLayer:           "Ground_Level",
	Layers: []Layer{
		{Name: "2nd Floor", SVGLayer: "First_Floor", Extents: []Extent{{Height: [2]float64{25, 34}, Bounds: [][2][2]float64{{{120, 218}, {-222, -327}}}}}},
		{Name: "3rd Floor", SVGLayer: "Second_Floor", Extents: []Extent{{Height: [2]float64{34, 1000}, Bounds: [][2][2]float64{{{120, 218}, {-222, -327}}}}}},
	},
}

func near(t *testing.T, name string, got, want, eps float64) {
	t.Helper()
	if math.Abs(got-want) > eps {
		t.Errorf("%s = %.6f, want %.6f", name, got, want)
	}
}

func TestProjectInterchange(t *testing.T) {
	px, py := Project(ic, -51.12, 47.90)
	near(t, "px", px, 164.1468, 1e-6)
	near(t, "py", py, 147.2935, 1e-6)
}

func TestProjectNoRotation(t *testing.T) {
	px, py := Project(MapDef{Transform: []float64{1, 0, 1, 0}}, 10, 20)
	near(t, "px", px, 10, 1e-9)
	near(t, "py", py, -20, 1e-9)
}

func TestImageRect(t *testing.T) {
	r := ImageRect(ic)
	near(t, "X", r.X, -7.87, 1e-6)
	near(t, "Y", r.Y, 17.47, 1e-6)
	near(t, "W", r.W, 273.215, 1e-6)
	near(t, "H", r.H, 230.02, 1e-6)
}

func TestLayerOf(t *testing.T) {
	cases := []struct {
		p    Vec3
		want string
	}{
		{Vec3{0, 30, 0}, "First_Floor"},
		{Vec3{0, 25, 0}, "First_Floor"},
		{Vec3{0, 34, 0}, "Second_Floor"},
		{Vec3{0, 40, 0}, "Second_Floor"},
		{Vec3{0, 22, 0}, "Ground_Level"},
		{Vec3{-312.74, 25.2, -111.95}, "Ground_Level"}, // вне прямоугольника ТЦ
	}
	for _, c := range cases {
		if got := LayerOf(ic, c.p); got != c.want {
			t.Errorf("LayerOf(%v) = %q, want %q", c.p, got, c.want)
		}
	}
}

// У Завода этажи заданы только диапазоном высот — без прямоугольников.
func TestLayerOfNoBounds(t *testing.T) {
	f := MapDef{SVGLayer: "Ground_Floor", Layers: []Layer{
		{SVGLayer: "Second_Floor", Extents: []Extent{{Height: [2]float64{3, 6}}}},
		{SVGLayer: "Basement", Extents: []Extent{{Height: [2]float64{-10000, -1}}}},
	}}
	if got := LayerOf(f, Vec3{10, 4, 10}); got != "Second_Floor" {
		t.Errorf("y=4 → %q, want Second_Floor", got)
	}
	if got := LayerOf(f, Vec3{10, -3, 10}); got != "Basement" {
		t.Errorf("y=-3 → %q, want Basement", got)
	}
	if got := LayerOf(f, Vec3{10, 1, 10}); got != "Ground_Floor" {
		t.Errorf("y=1 → %q, want Ground_Floor", got)
	}
}

func TestForward(t *testing.T) {
	fx, fz := Forward(0, 0.70711, 0, 0.70711)
	near(t, "fx", fx, 1, 1e-4)
	near(t, "fz", fz, 0, 1e-4)
	fx, fz = Forward(0, 0, 0, 1)
	near(t, "fx", fx, 0, 1e-12)
	near(t, "fz", fz, 1, 1e-12)
}

func TestLayerOfSpan(t *testing.T) {
	mall := [][2][2]float64{{{120, 218}, {-222, -327}}}
	ic := MapDef{SVGLayer: "Ground_Level", Layers: []Layer{
		{Name: "2nd Floor", SVGLayer: "First_Floor", Extents: []Extent{{Height: [2]float64{25, 34}, Bounds: mall}}},
		{Name: "3rd Floor", SVGLayer: "Second_Floor", Extents: []Extent{{Height: [2]float64{34, 1000}, Bounds: mall}}},
	}}
	factory := MapDef{SVGLayer: "Ground_Floor", Layers: []Layer{
		{Name: "3rd Floor", SVGLayer: "Third_Floor", Extents: []Extent{{Height: [2]float64{6, 10000}}}},
		{Name: "Underground", SVGLayer: "Underground_Level", Extents: []Extent{{Height: [2]float64{-1000, 0.5}}}},
	}}
	cases := []struct {
		name              string
		md                MapDef
		x, z, bottom, top float64
		want              string
	}{
		{"магазин 2-го этажа ТЦ", ic, -34, -235, 25, 33, "First_Floor"},
		{"магазин 3-го этажа ТЦ", ic, -34, -235, 34, 999, "Second_Floor"},
		{"земля", ic, -34, -235, -1, 10, "Ground_Level"},
		{"вне прямоугольника этажа", ic, 300, 300, 25, 33, "Ground_Level"},
		{"верх включительно", factory, 0, 0, 6, 10000, "Third_Floor"},
		{"диапазон шире любого этажа (санаторий Берега)", factory, 0, 0, -100, 100, "Ground_Floor"},
	}
	for _, c := range cases {
		if got := LayerOfSpan(c.md, c.x, c.z, c.bottom, c.top); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
