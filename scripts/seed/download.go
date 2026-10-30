package main

import (
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

// mode скачивает файлы режима ("pve" | "regular"). Названия предметов обязательны:
// без них у друга не заработает поиск лута.
func (d *downloader) mode(dir, mode string) error {
	for _, f := range []string{"maps", "maps_ru", "items_ru"} {
		name := mode + "_" + f + ".json"
		if err := d.get(d.jsonBase+"/"+mode+"/"+f, filepath.Join(dir, name)); err != nil {
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
	if err := d.get(primary, dst); err != nil {
		if err2 := d.get(d.svgFallback+"/"+filepath.Base(dst), dst); err2 != nil {
			return fmt.Errorf("SVG %s: %v; запасной адрес: %v", filepath.Base(dst), err, err2)
		}
	}
	return nil
}

func (d *downloader) get(url, dst string) error {
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
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}
