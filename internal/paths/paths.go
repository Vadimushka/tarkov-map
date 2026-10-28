// Package paths находит папку игры и папку скриншотов.
package paths

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Vadimushka/tarkov-map/internal/gamelog"
)

var vdfPathRe = regexp.MustCompile(`"path"\s*"((?:[^"\\]|\\.)*)"`)

// ParseLibraryFolders достаёт пути библиотек из steamapps\libraryfolders.vdf.
func ParseLibraryFolders(vdf string) []string {
	var out []string
	for _, m := range vdfPathRe.FindAllStringSubmatch(vdf, -1) {
		out = append(out, strings.ReplaceAll(m[1], `\\`, `\`))
	}
	return out
}

// ChooseGameDir выбирает кандидата, в чьём Logs самая свежая сессия.
// Так отсекаются остатки старых установок.
func ChooseGameDir(candidates []string) string {
	var best string
	var bestMod int64
	for _, c := range candidates {
		s, err := gamelog.LatestSession(filepath.Join(c, "Logs"))
		if err != nil {
			continue
		}
		info, err := os.Stat(s)
		if err != nil {
			continue
		}
		if m := info.ModTime().UnixNano(); best == "" || m > bestMod {
			best, bestMod = c, m
		}
	}
	return best
}
