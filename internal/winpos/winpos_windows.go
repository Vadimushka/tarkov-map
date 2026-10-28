package winpos

import (
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/Vadimushka/tarkov-map/internal/settings"
)

const windowClass = "wailsWindow" // класс главного окна Wails

var (
	user32                 = windows.NewLazySystemDLL("user32.dll")
	procGetWindowPlacement = user32.NewProc("GetWindowPlacement")
	procSetWindowPlacement = user32.NewProc("SetWindowPlacement")
	procMonitorFromRect    = user32.NewProc("MonitorFromRect")
)

// mainWindow ищет окно Wails своего процесса.
func mainWindow() windows.HWND {
	pid := uint32(os.Getpid())
	var found windows.HWND
	cb := syscall.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
		var p uint32
		if _, err := windows.GetWindowThreadProcessId(h, &p); err != nil || p != pid {
			return 1
		}
		buf := make([]uint16, 64)
		if n, err := windows.GetClassName(h, &buf[0], int32(len(buf))); err == nil && windows.UTF16ToString(buf[:n]) == windowClass {
			found = h
			return 0
		}
		return 1
	})
	_ = windows.EnumWindows(cb, nil) // остановка перебора возвращает «ошибку»
	return found
}

// Get возвращает положение окна; false — окна нет.
func Get() (settings.WindowPos, bool) {
	h := mainWindow()
	if h == 0 {
		return settings.WindowPos{}, false
	}
	p := placement{Length: placementSize}
	if r, _, _ := procGetWindowPlacement.Call(uintptr(h), uintptr(unsafe.Pointer(&p))); r == 0 {
		return settings.WindowPos{}, false
	}
	return fromPlacement(p), true
}

// Apply ставит окно на сохранённое место и показывает его (обычным или развёрнутым).
// false — окна нет, размер негодный или прямоугольник не попадает ни на один монитор
// (например, второй монитор отключили).
func Apply(w settings.WindowPos) bool {
	if !valid(w) {
		return false
	}
	h := mainWindow()
	if h == 0 {
		return false
	}
	p := toPlacement(w)
	const monitorDefaultToNull = 0
	if m, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&p.Normal)), monitorDefaultToNull); m == 0 {
		return false
	}
	r, _, _ := procSetWindowPlacement.Call(uintptr(h), uintptr(unsafe.Pointer(&p)))
	return r != 0
}
