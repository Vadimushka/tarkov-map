package gamelog

import (
	"os"
	"testing"
)

// Состояние заданий по настоящим логам игры: TM_LOGS=<папка Logs>.
func TestScanRealLogs(t *testing.T) {
	dir := os.Getenv("TM_LOGS")
	if dir == "" {
		t.Skip("только с TM_LOGS")
	}
	for mode, qs := range ScanQuests(dir) {
		n := map[string]int{}
		for id, st := range qs {
			n[st]++
			t.Logf("%s %s %s", mode, id, st)
		}
		t.Logf("%s: взято %d, сдано %d, провалено %d", mode, n["start"], n["done"], n["fail"])
	}
}
