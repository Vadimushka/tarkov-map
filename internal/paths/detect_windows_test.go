//go:build windows

package paths

import (
	"os"
	"testing"
)

// Проверка на реальном ПК владельца: TM_REAL=1 scripts/wintest.sh internal/paths
func TestDetectThisPC(t *testing.T) {
	if os.Getenv("TM_REAL") != "1" {
		t.Skip("только с TM_REAL=1")
	}
	game, shots := Detect()
	if game != `C:\SteamLibrary\steamapps\common\Escape from Tarkov\build` {
		t.Errorf("game = %q", game)
	}
	if st, err := os.Stat(shots); err != nil || !st.IsDir() {
		t.Errorf("shots = %q (%v)", shots, err)
	}
	t.Logf("game=%s shots=%s", game, shots)
}
