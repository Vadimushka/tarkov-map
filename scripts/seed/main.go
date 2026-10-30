// Команда seed собирает снимок данных tarkov.dev, который встраивается в exe:
// карты и метки обоих режимов, русские названия и все SVG-карты.
//
//	go run ./scripts/seed            # → assets/data.zip
//
// Запускать перед сборкой exe для передачи, чтобы данные в нём были свежими.
package main

import (
	"archive/zip"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Vadimushka/tarkov-map/internal/data"
)

func main() {
	out := flag.String("o", "assets/data.zip", "куда записать снимок")
	flag.Parse()
	dir, err := os.MkdirTemp("", "tarkov-seed-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	s := data.NewStore(dir, &http.Client{Timeout: 2 * time.Minute}, time.Now)
	for _, mode := range []string{"pve", "regular"} {
		if err := s.Fetch(mode, true); err != nil {
			log.Fatalf("%s: %v", mode, err)
		}
		// без названий предметов поиск лута у друга не заработает — снимок неполный
		if _, err := os.Stat(filepath.Join(dir, mode+"_items_ru.json")); err != nil {
			log.Fatalf("%s: названия предметов не скачались", mode)
		}
	}
	if err := s.LoadCached("pve"); err != nil {
		log.Fatal(err)
	}
	for _, d := range s.SVGDefs() {
		if err := s.RefreshSVG(d); err != nil {
			log.Fatal(err)
		}
	}
	if err := writeZip(dir, *out); err != nil {
		log.Fatal(err)
	}
	st, _ := os.Stat(*out)
	log.Printf("%s: %d КБ", *out, st.Size()/1024)
}

// writeZip пакует dir с путями через «/» и временем файлов: по нему Store решает,
// новее ли снимок кэша.
func writeZip(dir, out string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	err = filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		w, err := zw.CreateHeader(&zip.FileHeader{Name: filepath.ToSlash(rel), Method: zip.Deflate, Modified: info.ModTime()})
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	})
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}
