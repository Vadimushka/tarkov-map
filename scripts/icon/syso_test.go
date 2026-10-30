package main

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"image/png"
	"testing"
)

// entry ищет запись каталога ресурсов с данным id; dir — смещение каталога в секции.
func entry(t *testing.T, sec []byte, dir int, id uint32) (target int, sub bool) {
	t.Helper()
	le := binary.LittleEndian
	named, ids := int(le.Uint16(sec[dir+12:])), int(le.Uint16(sec[dir+14:]))
	for i := 0; i < named+ids; i++ {
		e := dir + 16 + 8*i
		if le.Uint32(sec[e:]) == id {
			v := le.Uint32(sec[e+4:])
			return int(v &^ (1 << 31)), v&(1<<31) != 0
		}
	}
	t.Fatalf("нет записи %d в каталоге @%d", id, dir)
	return 0, false
}

// resource возвращает данные ресурса type/id (язык — первый); OffsetToData до компоновки —
// смещение в секции, его и правит перемещение.
func resource(t *testing.T, sec []byte, typ, id uint32) []byte {
	t.Helper()
	le := binary.LittleEndian
	d, sub := entry(t, sec, 0, typ)
	if !sub {
		t.Fatal("тип должен вести в подкаталог")
	}
	if d, sub = entry(t, sec, d, id); !sub {
		t.Fatal("id должен вести в подкаталог языков")
	}
	lang := int(le.Uint32(sec[d+16+4:]))
	off, size := int(le.Uint32(sec[lang:])), int(le.Uint32(sec[lang+4:]))
	return sec[off : off+size]
}

func TestIconSyso(t *testing.T) {
	sizes := []int{16, 32, 256}
	var pngs [][]byte
	for _, s := range sizes {
		var b bytes.Buffer
		if err := png.Encode(&b, render(s)); err != nil {
			t.Fatal(err)
		}
		pngs = append(pngs, b.Bytes())
	}
	f, err := pe.NewFile(bytes.NewReader(iconSyso(3, sizes, pngs)))
	if err != nil {
		t.Fatal(err)
	}
	if f.Machine != pe.IMAGE_FILE_MACHINE_AMD64 || len(f.Sections) != 1 || f.Sections[0].Name != ".rsrc" {
		t.Fatalf("заголовок: machine %#x, секции %d", f.Machine, len(f.Sections))
	}
	if len(f.COFFSymbols) != 1 {
		t.Errorf("символов %d, нужен один — .rsrc", len(f.COFFSymbols))
	}
	s := f.Sections[0]
	if len(s.Relocs) != len(sizes)+1 {
		t.Fatalf("перемещений %d, нужно по одному на ресурс", len(s.Relocs))
	}
	for _, r := range s.Relocs {
		if r.Type != 3 || r.SymbolTableIndex != 0 {
			t.Errorf("перемещение %+v: нужно IMAGE_REL_AMD64_ADDR32NB к символу 0", r)
		}
	}
	sec, err := s.Data()
	if err != nil {
		t.Fatal(err)
	}
	grp := resource(t, sec, 14, 3) // RT_GROUP_ICON с id, который ищет Wails
	le := binary.LittleEndian
	if le.Uint16(grp[2:]) != 1 || int(le.Uint16(grp[4:])) != len(sizes) {
		t.Fatalf("заголовок группы % x", grp[:6])
	}
	for i, size := range sizes {
		e := grp[6+14*i:]
		if w := int(e[0]); w != size%256 || e[1] != e[0] {
			t.Errorf("иконка %d: размер %d×%d, нужен %d (256 пишется как 0)", i, e[0], e[1], size)
		}
		data := resource(t, sec, 3, uint32(le.Uint16(e[12:])))
		if int(le.Uint32(e[8:])) != len(data) || !bytes.Equal(data, pngs[i]) {
			t.Errorf("иконка %d: данные не совпадают с PNG", i)
		}
	}
}

func TestRender(t *testing.T) {
	img := render(64)
	if a := img.NRGBAAt(0, 0).A; a != 0 {
		t.Errorf("угол должен быть прозрачным, альфа %d", a)
	}
	c := img.NRGBAAt(int(cx/4), int(cy/4)) // центр метки
	if c.R < 200 || c.G > 120 || c.A != 255 {
		t.Errorf("центр метки должен быть красным: %+v", c)
	}
}
