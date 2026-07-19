package browserengine

import "fmt"

type NativeSurfaceBounds struct {
	X              float64 `json:"x"`
	Y              float64 `json:"y"`
	Width          float64 `json:"width"`
	Height         float64 `json:"height"`
	ViewportWidth  float64 `json:"viewportWidth"`
	ViewportHeight float64 `json:"viewportHeight"`
}

type nativeWindowInsets struct {
	Left   float64
	Top    float64
	Right  float64
	Bottom float64
}

type nativeBrowserSurface interface {
	Supported() bool
	Attach(processID int) error
	Show(bounds NativeSurfaceBounds, insets nativeWindowInsets) error
	Hide() error
	Close()
}

func validateNativeSurfaceBounds(bounds NativeSurfaceBounds) error {
	if bounds.Width < 1 || bounds.Height < 1 {
		return fmt.Errorf("浏览器显示区域尺寸无效")
	}
	if bounds.ViewportWidth < 1 || bounds.ViewportHeight < 1 {
		return fmt.Errorf("应用窗口尺寸无效")
	}
	return nil
}
