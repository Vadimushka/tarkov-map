//go:build !windows

package paths

// Detect вне Windows ничего не находит: игра есть только под Windows.
func Detect() (gameDir, shotsDir string) {
	return "", ""
}
