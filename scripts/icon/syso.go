package main

import "encoding/binary"

type res struct {
	typ, id uint16
	data    []byte
}

// iconSyso собирает COFF-объект для go build (GOOS=windows подхватывает *.syso пакета сам):
// RT_ICON 1…N с PNG-картинками и RT_GROUP_ICON groupID, который на них ссылается.
func iconSyso(groupID uint16, sizes []int, pngs [][]byte) []byte {
	le := binary.LittleEndian
	grp := le.AppendUint16(nil, 0)
	grp = le.AppendUint16(grp, 1) // иконка, не курсор
	grp = le.AppendUint16(grp, uint16(len(pngs)))
	var rs []res
	for i, p := range pngs {
		w := uint8(sizes[i]) // 256 пишется как 0
		grp = append(grp, w, w, 0, 0)
		grp = le.AppendUint16(grp, 1)  // planes
		grp = le.AppendUint16(grp, 32) // бит на пиксель
		grp = le.AppendUint32(grp, uint32(len(p)))
		grp = le.AppendUint16(grp, uint16(i+1))
		rs = append(rs, res{3, uint16(i + 1), p})
	}
	return coff(append(rs, res{14, groupID, grp}))
}

// coff раскладывает ресурсы деревом тип → id → язык (нейтральный). rs отсортированы по
// типу, внутри типа — по id: Windows ищет записи двоичным поиском.
func coff(rs []res) []byte {
	le := binary.LittleEndian
	var types []uint16
	count := map[uint16]int{}
	for _, r := range rs {
		if count[r.typ] == 0 {
			types = append(types, r.typ)
		}
		count[r.typ]++
	}
	dirSize := func(n int) int { return 16 + 8*n }
	off := dirSize(len(types))
	typeOff := map[uint16]int{}
	for _, t := range types {
		typeOff[t] = off
		off += dirSize(count[t])
	}
	langOff, entryOff, dataOff := make([]int, len(rs)), make([]int, len(rs)), make([]int, len(rs))
	for i := range rs {
		langOff[i] = off
		off += dirSize(1)
	}
	for i := range rs {
		entryOff[i] = off
		off += 16
	}
	for i, r := range rs {
		off = (off + 7) &^ 7
		dataOff[i] = off
		off += len(r.data)
	}
	raw := make([]byte, (off+7)&^7)

	dir := func(at, n int) int { // каталог только с id-записями; вернуть начало записей
		le.PutUint16(raw[at+14:], uint16(n))
		return at + 16
	}
	put := func(at int, id uint32, target int, sub bool) {
		v := uint32(target)
		if sub {
			v |= 1 << 31
		}
		le.PutUint32(raw[at:], id)
		le.PutUint32(raw[at+4:], v)
	}
	p := dir(0, len(types))
	for _, t := range types {
		put(p, uint32(t), typeOff[t], true)
		p += 8
	}
	next := map[uint16]int{}
	var relocs []byte
	for i, r := range rs {
		put(dir(typeOff[r.typ], count[r.typ])+8*next[r.typ], uint32(r.id), langOff[i], true)
		next[r.typ]++
		put(dir(langOff[i], 1), 0, entryOff[i], false)
		// смещение в секции; перемещение ADDR32NB превращает его в RVA при компоновке
		le.PutUint32(raw[entryOff[i]:], uint32(dataOff[i]))
		le.PutUint32(raw[entryOff[i]+4:], uint32(len(r.data)))
		copy(raw[dataOff[i]:], r.data)
		relocs = le.AppendUint32(relocs, uint32(entryOff[i]))
		relocs = le.AppendUint32(relocs, 0) // символ .rsrc
		relocs = le.AppendUint16(relocs, 3) // IMAGE_REL_AMD64_ADDR32NB
	}

	const hdr = 20 + 40 // заголовок файла и одной секции
	relOff := hdr + len(raw)
	symOff := relOff + len(relocs)
	out := le.AppendUint16(nil, 0x8664) // AMD64
	out = le.AppendUint16(out, 1)       // секций
	out = le.AppendUint32(out, 0)       // время
	out = le.AppendUint32(out, uint32(symOff))
	out = le.AppendUint32(out, 1)      // символов
	out = le.AppendUint16(out, 0)      // optional header нет
	out = le.AppendUint16(out, 0x0004) // IMAGE_FILE_LINE_NUMS_STRIPPED
	out = append(out, ".rsrc\x00\x00\x00"...)
	out = le.AppendUint32(out, 0) // VirtualSize
	out = le.AppendUint32(out, 0) // VirtualAddress
	out = le.AppendUint32(out, uint32(len(raw)))
	out = le.AppendUint32(out, hdr)
	out = le.AppendUint32(out, uint32(relOff))
	out = le.AppendUint32(out, 0) // номера строк
	out = le.AppendUint16(out, uint16(len(rs)))
	out = le.AppendUint16(out, 0)
	out = le.AppendUint32(out, 0x40000040) // INITIALIZED_DATA | MEM_READ
	out = append(out, raw...)
	out = append(out, relocs...)
	out = append(out, ".rsrc\x00\x00\x00"...)
	out = le.AppendUint32(out, 0)  // значение
	out = le.AppendUint16(out, 1)  // секция 1
	out = le.AppendUint16(out, 0)  // тип
	out = append(out, 3, 0)        // IMAGE_SYM_CLASS_STATIC, без доп. записей
	return le.AppendUint32(out, 4) // таблица строк пуста: только её длина
}
