// Генератор иконки приложения: go run ./scripts/icon [-size N -o file.png].
// Тёмный скруглённый квадрат, сложенная втрое карта и метка «я здесь» с конусом взгляда —
// как на карте приложения. Сглаживание — суперсэмплинг 4×4.
package main

import (
	"bytes"
	"flag"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
)

type rgba struct{ r, g, b, a float64 } // без premultiply, компоненты 0…1

func hex(v uint32) rgba {
	return rgba{float64(v>>16&0xff) / 255, float64(v>>8&0xff) / 255, float64(v&0xff) / 255, 1}
}

// over кладёт c поверх dst с непрозрачностью alpha.
func over(dst, c rgba, alpha float64) rgba {
	a := alpha + dst.a*(1-alpha)
	if a == 0 {
		return rgba{}
	}
	mix := func(s, d float64) float64 { return (s*alpha + d*dst.a*(1-alpha)) / a }
	return rgba{mix(c.r, dst.r), mix(c.g, dst.g), mix(c.b, dst.b), a}
}

var (
	bg     = hex(0x14130f)
	paper  = hex(0xe0c27a)
	shade  = hex(0xc4a560) // средняя панель — на сгибе
	fold   = hex(0x9c7f45)
	marker = hex(0xff5d5d)
	white  = hex(0xffffff)
)

// Все размеры — в координатах иконки 256×256.
const (
	inset, corner = 8.0, 48.0
	mapL, mapR    = 44.0, 212.0
	mapH          = 120.0
	cx, cy        = 140.0, 140.0 // метка
	ring, dot     = 30.0, 23.0
	coneR         = 96.0
	coneDir       = -100 * math.Pi / 180 // вверх и чуть влево
	coneHalf      = 30 * math.Pi / 180
)

func inRoundRect(x, y float64) bool {
	lo, hi := inset+corner, 256-inset-corner
	dx := math.Max(0, math.Max(lo-x, x-hi))
	dy := math.Max(0, math.Max(lo-y, y-hi))
	return x >= inset && x <= 256-inset && y >= inset && y <= 256-inset && dx*dx+dy*dy <= corner*corner
}

// mapTop — верхний край сложенной карты: зигзаг по сгибам.
func mapTop(x float64) float64 {
	w := (mapR - mapL) / 3
	t := math.Mod(x-mapL, 2*w)
	if t > w {
		t = 2*w - t
	}
	return 62 + 16*t/w
}

func sample(x, y float64) rgba {
	var c rgba
	if !inRoundRect(x, y) {
		return c
	}
	c = bg
	if x >= mapL && x <= mapR {
		if top := mapTop(x); y >= top && y <= top+mapH {
			panel := int((x - mapL) / ((mapR - mapL) / 3))
			c = paper
			if panel == 1 {
				c = shade
			}
			for _, fx := range []float64{mapL + (mapR-mapL)/3, mapL + 2*(mapR-mapL)/3} {
				if math.Abs(x-fx) < 1.5 {
					c = fold
				}
			}
		}
	}
	dx, dy := x-cx, y-cy
	d := math.Hypot(dx, dy)
	if d <= coneR {
		diff := math.Abs(math.Remainder(math.Atan2(dy, dx)-coneDir, 2*math.Pi))
		if diff <= coneHalf {
			c = over(c, marker, 0.5*(1-d/coneR*0.6))
		}
	}
	if d <= ring {
		c = white
	}
	if d <= dot {
		c = marker
	}
	return c
}

// render рисует иконку стороной size: каждую сторону — заново, а не уменьшением большой,
// чтобы 16 px оставались резкими.
func render(size int) *image.NRGBA {
	const ss = 4
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	k := 256 / float64(size)
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var r, g, b, a float64 // premultiplied: среднее по подвыборкам
			for i := 0; i < ss; i++ {
				for j := 0; j < ss; j++ {
					c := sample((float64(px)+(float64(j)+0.5)/ss)*k, (float64(py)+(float64(i)+0.5)/ss)*k)
					r, g, b, a = r+c.r*c.a, g+c.g*c.a, b+c.b*c.a, a+c.a
				}
			}
			if a == 0 {
				continue
			}
			to8 := func(v float64) uint8 { return uint8(math.Round(v / a * 255)) }
			img.SetNRGBA(px, py, color.NRGBA{to8(r), to8(g), to8(b), uint8(math.Round(a / ss / ss * 255))})
		}
	}
	return img
}

func encode(size int) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, render(size)); err != nil {
		log.Fatal(err)
	}
	return b.Bytes()
}

// Без флагов пишет assets/appicon.png и ресурс rsrc_windows_amd64.syso (запускать из
// корня модуля); с -size — только PNG этого размера, для просмотра.
func main() {
	size := flag.Int("size", 0, "только PNG этой стороны (просмотр)")
	out := flag.String("o", "assets/appicon.png", "куда записать PNG")
	flag.Parse()

	if *size > 0 {
		must(os.WriteFile(*out, encode(*size), 0o644))
		return
	}
	sizes := []int{16, 20, 24, 32, 40, 48, 64, 256}
	var pngs [][]byte
	for _, s := range sizes {
		pngs = append(pngs, encode(s))
	}
	must(os.WriteFile(*out, pngs[len(pngs)-1], 0o644))
	// id 3 — под ним Wails ищет иконку окна (winc.AppIconID); Проводник берёт первую группу
	must(os.WriteFile("rsrc_windows_amd64.syso", iconSyso(3, sizes, pngs), 0o644))
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
