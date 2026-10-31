package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Vadimushka/tarkov-map/internal/data"
	"github.com/Vadimushka/tarkov-map/internal/mapgeo"
)

const assetsSVG = "https://assets.tarkov.dev/maps/svg"

// downloader качает данные tarkov.dev в папку снимка, по путям, которые читает data.Store.
type downloader struct {
	jsonBase, mapsJSON string
	svgBase            string // "" — брать svgPath из maps.json как есть
	svgFallback        string
	client             *http.Client
}

func newDownloader(client *http.Client) *downloader {
	return &downloader{
		jsonBase:    "https://json.tarkov.dev",
		mapsJSON:    "https://raw.githubusercontent.com/the-hideout/tarkov-dev/HEAD/src/data/maps.json",
		svgFallback: "https://raw.githubusercontent.com/the-hideout/tarkov-dev-svg-maps/HEAD",
		client:      client,
	}
}

// mode скачивает файлы режима ("pve" | "regular"). Названия предметов и задания обязательны:
// без них у друга не заработают поиск лута и квесты.
func (d *downloader) mode(dir, mode string) error {
	for _, f := range []string{"maps", "maps_ru", "items_ru", "tasks", "tasks_ru"} {
		name := mode + "_" + f + ".json"
		if err := d.save(d.jsonBase+"/"+mode+"/"+f, filepath.Join(dir, name), nil); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

// svg скачивает SVG-карту: основной адрес, затем запасной.
func (d *downloader) svg(dir string, def mapgeo.MapDef) error {
	dst := filepath.Join(dir, filepath.FromSlash(data.SVGName(def)))
	primary := def.SVGPath
	if d.svgBase != "" {
		primary = strings.Replace(primary, assetsSVG, d.svgBase, 1)
	}
	if err := d.save(primary, dst, isSVG); err != nil {
		if err2 := d.save(d.svgFallback+"/"+filepath.Base(dst), dst, isSVG); err2 != nil {
			return fmt.Errorf("SVG %s: %v; запасной адрес: %v", filepath.Base(dst), err, err2)
		}
	}
	return nil
}

// save скачивает url в dst; check, если задан, проверяет ответ до записи.
func (d *downloader) save(url, dst string, check func([]byte) error) error {
	resp, err := d.client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if check != nil {
		if err := check(b); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

// isSVG отсеивает ответы 200, которые не карта: страницы ошибок прокси и CDN.
func isSVG(b []byte) error {
	if !bytes.Contains(b[:min(len(b), 4096)], []byte("<svg")) {
		return errors.New("ответ — не SVG")
	}
	return nil
}
