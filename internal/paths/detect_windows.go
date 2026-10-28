//go:build windows

package paths

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Detect ищет папку игры (где лежит Logs) и папку скриншотов.
func Detect() (gameDir, shotsDir string) {
	var cands []string
	if p := regString(registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\EscapeFromTarkov`, "InstallLocation"); p != "" {
		cands = append(cands, p)
	}
	steam := regString(registry.CURRENT_USER, `Software\Valve\Steam`, "SteamPath")
	if steam == "" {
		steam = regString(registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Valve\Steam`, "InstallPath")
	}
	if steam != "" {
		libs := []string{filepath.FromSlash(steam)}
		if b, err := os.ReadFile(filepath.Join(steam, "steamapps", "libraryfolders.vdf")); err == nil {
			libs = append(libs, ParseLibraryFolders(string(b))...)
		}
		for _, l := range libs {
			cands = append(cands, filepath.Join(l, "steamapps", "common", "Escape from Tarkov", "build"))
		}
	}
	cands = append(cands, `C:\Battlestate Games\EFT`)
	gameDir = ChooseGameDir(cands)

	if docs, err := windows.KnownFolderPath(windows.FOLDERID_Documents, 0); err == nil {
		shotsDir = filepath.Join(docs, "Escape from Tarkov", "Screenshots")
	}
	return gameDir, shotsDir
}

func regString(root registry.Key, path, name string) string {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue(name)
	if err != nil {
		return ""
	}
	return v
}
