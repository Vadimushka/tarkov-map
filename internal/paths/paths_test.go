package paths

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

const vdf = `"libraryfolders"
{
	"0"
	{
		"path"		"C:\\Program Files (x86)\\Steam"
		"label"		""
	}
	"1"
	{
		"path"		"C:\\SteamLibrary"
	}
}
`

func TestParseLibraryFolders(t *testing.T) {
	got := ParseLibraryFolders(vdf)
	want := []string{`C:\Program Files (x86)\Steam`, `C:\SteamLibrary`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func mkLogs(t *testing.T, root, session string, mod time.Time) {
	t.Helper()
	d := filepath.Join(root, "Logs", session)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(d, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func TestChooseGameDir(t *testing.T) {
	base := t.TempDir()
	a, b, c := filepath.Join(base, "A"), filepath.Join(base, "B"), filepath.Join(base, "C")
	mkLogs(t, a, "log_old", time.Date(2025, 12, 26, 20, 3, 0, 0, time.Local))
	mkLogs(t, b, "log_new", time.Now())
	if err := os.MkdirAll(c, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ChooseGameDir([]string{a, b, c}); got != b {
		t.Errorf("ChooseGameDir = %q, want %q", got, b)
	}
	if got := ChooseGameDir(nil); got != "" {
		t.Errorf("ChooseGameDir(nil) = %q", got)
	}
}
