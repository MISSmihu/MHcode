//go:build windows

package browserengine

import (
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	gwlHwndParent = -8
	gwlStyle      = -16
	gwlExStyle    = -20

	wsPopup          = 0x80000000
	wsChild          = 0x40000000
	wsVisible        = 0x10000000
	wsClipSiblings   = 0x04000000
	wsClipChildren   = 0x02000000
	wsOverlappedWind = 0x00CF0000

	wsExToolWindow = 0x00000080
	wsExAppWindow  = 0x00040000

	swHide = 0

	swpNoSize       = 0x0001
	swpNoMove       = 0x0002
	swpNoZOrder     = 0x0004
	swpNoActivate   = 0x0010
	swpFrameChanged = 0x0020
	swpShowWindow   = 0x0040
	swpNoOwnerOrder = 0x0200
)

var (
	user32                 = syscall.NewLazyDLL("user32.dll")
	gdi32                  = syscall.NewLazyDLL("gdi32.dll")
	procEnumWindows        = user32.NewProc("EnumWindows")
	procGetWindowThreadPID = user32.NewProc("GetWindowThreadProcessId")
	procGetClassName       = user32.NewProc("GetClassNameW")
	procIsWindow           = user32.NewProc("IsWindow")
	procIsWindowVisible    = user32.NewProc("IsWindowVisible")
	procGetWindowRect      = user32.NewProc("GetWindowRect")
	procGetClientRect      = user32.NewProc("GetClientRect")
	procClientToScreen     = user32.NewProc("ClientToScreen")
	procSetWindowPos       = user32.NewProc("SetWindowPos")
	procShowWindow         = user32.NewProc("ShowWindow")
	procSetWindowRgn       = user32.NewProc("SetWindowRgn")
	procGetWindowLong      = user32.NewProc("GetWindowLongW")
	procSetWindowLong      = user32.NewProc("SetWindowLongW")
	procGetWindowLongPtr   = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtr   = user32.NewProc("SetWindowLongPtrW")
	procCreateRectRgn      = gdi32.NewProc("CreateRectRgn")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
)

type nativeRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type nativePoint struct {
	X int32
	Y int32
}

type nativeSurfacePlacement struct {
	X             int
	Y             int
	WindowWidth   int
	WindowHeight  int
	LeftInset     int
	TopInset      int
	ContentWidth  int
	ContentHeight int
}

func (p nativeSurfacePlacement) sameClip(other nativeSurfacePlacement) bool {
	return p.WindowWidth == other.WindowWidth &&
		p.WindowHeight == other.WindowHeight &&
		p.LeftInset == other.LeftInset &&
		p.TopInset == other.TopInset &&
		p.ContentWidth == other.ContentWidth &&
		p.ContentHeight == other.ContentHeight
}

type windowsNativeBrowserSurface struct {
	mu              sync.Mutex
	processID       uint32
	mainWindow      uintptr
	browserWindow   uintptr
	originalStyle   uintptr
	originalExStyle uintptr
	originalOwner   uintptr
	visible         bool
	bounds          NativeSurfaceBounds
	insets          nativeWindowInsets
	placement       nativeSurfacePlacement
	positioned      bool
	positionStop    chan struct{}
	positionDone    chan struct{}
}

func newNativeBrowserSurface() nativeBrowserSurface { return &windowsNativeBrowserSurface{} }

func (*windowsNativeBrowserSurface) Supported() bool { return true }

func (s *windowsNativeBrowserSurface) Attach(processID int) error {
	s.Close()
	if processID <= 0 {
		return fmt.Errorf("浏览器进程无效")
	}
	mainWindow, err := waitForNativeWindow(uint32(os.Getpid()), isWailsWindow, 4*time.Second)
	if err != nil {
		return fmt.Errorf("查找 MHcode 主窗口失败: %w", err)
	}
	browserWindow, err := waitForNativeWindow(uint32(processID), isChromiumWindow, 8*time.Second)
	if err != nil {
		return fmt.Errorf("查找浏览器原生窗口失败: %w", err)
	}

	originalStyle := nativeGetWindowLong(browserWindow, gwlStyle)
	originalExStyle := nativeGetWindowLong(browserWindow, gwlExStyle)
	originalOwner := nativeGetWindowLong(browserWindow, gwlHwndParent)
	procShowWindow.Call(browserWindow, swHide)

	style := originalStyle
	style &^= wsOverlappedWind | wsChild
	style |= wsPopup | wsVisible | wsClipSiblings | wsClipChildren
	nativeSetWindowLong(browserWindow, gwlStyle, style)
	exStyle := originalExStyle
	exStyle &^= wsExAppWindow
	exStyle |= wsExToolWindow
	nativeSetWindowLong(browserWindow, gwlExStyle, exStyle)
	nativeSetWindowLong(browserWindow, gwlHwndParent, mainWindow)

	s.mu.Lock()
	s.processID = uint32(processID)
	s.mainWindow = mainWindow
	s.browserWindow = browserWindow
	s.originalStyle = originalStyle
	s.originalExStyle = originalExStyle
	s.originalOwner = originalOwner
	s.positionStop = make(chan struct{})
	s.positionDone = make(chan struct{})
	stop := s.positionStop
	done := s.positionDone
	s.mu.Unlock()
	go s.monitorNativePosition(stop, done)
	return nil
}

func (s *windowsNativeBrowserSurface) Show(bounds NativeSurfaceBounds, insets nativeWindowInsets) error {
	if err := validateNativeSurfaceBounds(bounds); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mainWindow == 0 || s.browserWindow == 0 || !nativeIsWindow(s.browserWindow) {
		return fmt.Errorf("浏览器原生窗口尚未就绪")
	}
	placement, err := calculateNativeSurfacePlacement(s.mainWindow, bounds, insets)
	if err != nil {
		return err
	}
	if err := applyNativeSurfacePlacement(s.browserWindow, placement, true); err != nil {
		return err
	}
	s.visible = true
	s.bounds = bounds
	s.insets = insets
	s.placement = placement
	s.positioned = true
	return nil
}

func (s *windowsNativeBrowserSurface) Hide() error {
	s.mu.Lock()
	browserWindow := s.browserWindow
	s.visible = false
	s.positioned = false
	s.mu.Unlock()
	if browserWindow == 0 || !nativeIsWindow(browserWindow) {
		return nil
	}
	procShowWindow.Call(browserWindow, swHide)
	return nil
}

func (s *windowsNativeBrowserSurface) Close() {
	s.mu.Lock()
	stop := s.positionStop
	done := s.positionDone
	browserWindow := s.browserWindow
	originalStyle := s.originalStyle
	originalExStyle := s.originalExStyle
	originalOwner := s.originalOwner
	s.visible = false
	s.positioned = false
	s.positionStop = nil
	s.positionDone = nil
	s.processID = 0
	s.mainWindow = 0
	s.browserWindow = 0
	s.originalStyle = 0
	s.originalExStyle = 0
	s.originalOwner = 0
	s.bounds = NativeSurfaceBounds{}
	s.insets = nativeWindowInsets{}
	s.placement = nativeSurfacePlacement{}
	s.mu.Unlock()
	if stop != nil {
		close(stop)
	}
	if done != nil {
		<-done
	}
	if browserWindow == 0 || !nativeIsWindow(browserWindow) {
		return
	}
	procShowWindow.Call(browserWindow, swHide)
	procSetWindowRgn.Call(browserWindow, 0, 1)
	nativeSetWindowLong(browserWindow, gwlHwndParent, originalOwner)
	nativeSetWindowLong(browserWindow, gwlExStyle, originalExStyle)
	nativeSetWindowLong(browserWindow, gwlStyle, originalStyle)
	procSetWindowPos.Call(
		browserWindow,
		0,
		0,
		0,
		0,
		0,
		uintptr(swpNoSize|swpNoMove|swpNoZOrder|swpNoActivate|swpFrameChanged|swpNoOwnerOrder),
	)
}

func (s *windowsNativeBrowserSurface) monitorNativePosition(stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(16 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			s.syncNativePosition()
		}
	}
}

func (s *windowsNativeBrowserSurface) syncNativePosition() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.visible || s.mainWindow == 0 || s.browserWindow == 0 || !nativeIsWindow(s.browserWindow) {
		return
	}
	placement, err := calculateNativeSurfacePlacement(s.mainWindow, s.bounds, s.insets)
	if err != nil || s.positioned && placement == s.placement {
		return
	}
	if s.positioned && placement.sameClip(s.placement) {
		flags := uintptr(swpNoSize | swpNoZOrder | swpNoActivate | swpNoOwnerOrder)
		if result, _, setErr := procSetWindowPos.Call(
			s.browserWindow,
			0,
			uintptr(placement.X),
			uintptr(placement.Y),
			0,
			0,
			flags,
		); result == 0 {
			_ = setErr
			return
		}
	} else if err := applyNativeSurfacePlacement(s.browserWindow, placement, true); err != nil {
		return
	}
	s.placement = placement
	s.positioned = true
}

func calculateNativeSurfacePlacement(
	mainWindow uintptr,
	bounds NativeSurfaceBounds,
	insets nativeWindowInsets,
) (nativeSurfacePlacement, error) {
	var client nativeRect
	if result, _, callErr := procGetClientRect.Call(mainWindow, uintptr(unsafe.Pointer(&client))); result == 0 {
		return nativeSurfacePlacement{}, fmt.Errorf("读取 MHcode 窗口尺寸失败: %w", callErr)
	}
	origin := nativePoint{}
	if result, _, callErr := procClientToScreen.Call(mainWindow, uintptr(unsafe.Pointer(&origin))); result == 0 {
		return nativeSurfacePlacement{}, fmt.Errorf("读取 MHcode 窗口位置失败: %w", callErr)
	}
	clientWidth := float64(client.Right - client.Left)
	clientHeight := float64(client.Bottom - client.Top)
	scaleX := clientWidth / bounds.ViewportWidth
	scaleY := clientHeight / bounds.ViewportHeight
	if scaleX <= 0 || scaleY <= 0 {
		return nativeSurfacePlacement{}, fmt.Errorf("浏览器原生窗口缩放比例无效")
	}

	leftInset := int(math.Round(math.Max(0, insets.Left*scaleX)))
	rightInset := int(math.Round(math.Max(0, insets.Right*scaleX)))
	topInset := int(math.Round(math.Max(0, insets.Top*scaleY)))
	bottomInset := int(math.Round(math.Max(0, insets.Bottom*scaleY)))
	contentWidth := max(1, int(math.Round(bounds.Width*scaleX)))
	contentHeight := max(1, int(math.Round(bounds.Height*scaleY)))
	return nativeSurfacePlacement{
		X:             int(origin.X) + int(math.Round(bounds.X*scaleX)) - leftInset,
		Y:             int(origin.Y) + int(math.Round(bounds.Y*scaleY)) - topInset,
		WindowWidth:   contentWidth + leftInset + rightInset,
		WindowHeight:  contentHeight + topInset + bottomInset,
		LeftInset:     leftInset,
		TopInset:      topInset,
		ContentWidth:  contentWidth,
		ContentHeight: contentHeight,
	}, nil
}

func applyNativeSurfacePlacement(browserWindow uintptr, placement nativeSurfacePlacement, show bool) error {
	region, _, callErr := procCreateRectRgn.Call(
		uintptr(placement.LeftInset),
		uintptr(placement.TopInset),
		uintptr(placement.LeftInset+placement.ContentWidth),
		uintptr(placement.TopInset+placement.ContentHeight),
	)
	if region == 0 {
		return fmt.Errorf("创建浏览器裁剪区域失败: %w", callErr)
	}
	if result, _, setErr := procSetWindowRgn.Call(browserWindow, region, 1); result == 0 {
		procDeleteObject.Call(region)
		return fmt.Errorf("裁剪浏览器原生窗口失败: %w", setErr)
	}
	flags := uintptr(swpNoActivate | swpFrameChanged | swpNoOwnerOrder)
	if show {
		flags |= swpShowWindow
	}
	if result, _, setErr := procSetWindowPos.Call(
		browserWindow,
		0,
		uintptr(placement.X),
		uintptr(placement.Y),
		uintptr(placement.WindowWidth),
		uintptr(placement.WindowHeight),
		flags,
	); result == 0 {
		return fmt.Errorf("调整浏览器原生窗口失败: %w", setErr)
	}
	return nil
}

func waitForNativeWindow(processID uint32, accept func(uintptr, string) bool, timeout time.Duration) (uintptr, error) {
	deadline := time.Now().Add(timeout)
	for {
		if window := findNativeWindow(processID, accept); window != 0 {
			return window, nil
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("未找到进程 %d 的窗口", processID)
		}
		time.Sleep(80 * time.Millisecond)
	}
}

func findNativeWindow(processID uint32, accept func(uintptr, string) bool) uintptr {
	var best uintptr
	var bestArea int64
	callback := windows.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		var candidatePID uint32
		procGetWindowThreadPID.Call(hwnd, uintptr(unsafe.Pointer(&candidatePID)))
		if candidatePID != processID {
			return 1
		}
		className := nativeWindowClass(hwnd)
		if !accept(hwnd, className) {
			return 1
		}
		var rect nativeRect
		if result, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect))); result == 0 {
			return 1
		}
		area := int64(rect.Right-rect.Left) * int64(rect.Bottom-rect.Top)
		if area > bestArea {
			best = hwnd
			bestArea = area
		}
		return 1
	})
	procEnumWindows.Call(callback, 0)
	return best
}

func isWailsWindow(hwnd uintptr, className string) bool {
	return strings.EqualFold(className, "wailsWindow") && nativeIsWindowVisible(hwnd)
}

func isChromiumWindow(hwnd uintptr, className string) bool {
	return strings.HasPrefix(className, "Chrome_WidgetWin_") && nativeIsWindowVisible(hwnd)
}

func nativeWindowClass(hwnd uintptr) string {
	buffer := make([]uint16, 128)
	length, _, _ := procGetClassName.Call(hwnd, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	if length == 0 {
		return ""
	}
	return windows.UTF16ToString(buffer[:length])
}

func nativeIsWindow(hwnd uintptr) bool {
	result, _, _ := procIsWindow.Call(hwnd)
	return result != 0
}

func nativeIsWindowVisible(hwnd uintptr) bool {
	result, _, _ := procIsWindowVisible.Call(hwnd)
	return result != 0
}

func nativeGetWindowLong(hwnd uintptr, index int32) uintptr {
	if unsafe.Sizeof(uintptr(0)) == 8 {
		result, _, _ := procGetWindowLongPtr.Call(hwnd, uintptr(index))
		return result
	}
	result, _, _ := procGetWindowLong.Call(hwnd, uintptr(index))
	return result
}

func nativeSetWindowLong(hwnd uintptr, index int32, value uintptr) {
	if unsafe.Sizeof(uintptr(0)) == 8 {
		procSetWindowLongPtr.Call(hwnd, uintptr(index), value)
		return
	}
	procSetWindowLong.Call(hwnd, uintptr(index), value)
}
