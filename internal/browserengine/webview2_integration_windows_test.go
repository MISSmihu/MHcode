//go:build windows

package browserengine

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"golang.org/x/sys/windows"
)

var (
	testCreateWindowEx = syscall.NewLazyDLL("user32.dll").NewProc("CreateWindowExW")
	testDestroyWindow  = syscall.NewLazyDLL("user32.dll").NewProc("DestroyWindow")
)

// TestEmbeddedWebView2CDP exercises the real WebView2 runtime only when
// explicitly requested. It isolates each run in a temporary user data folder
// and helps catch runtime upgrades that break the private CDP connection.
func TestEmbeddedWebView2CDP(t *testing.T) {
	if os.Getenv("MHCODE_WEBVIEW2_INTEGRATION") != "1" {
		t.Skip("set MHCODE_WEBVIEW2_INTEGRATION=1 to run the WebView2 integration test")
	}

	hwnd, closeWindow := startWebView2TestWindow(t)
	defer closeWindow()

	port, err := reserveLoopbackPort()
	if err != nil {
		t.Fatal(err)
	}
	profileDir, err := os.MkdirTemp("", "mhcode-webview2-integration-")
	if err != nil {
		t.Fatal(err)
	}
	defer removeWebView2TestProfile(t, profileDir)
	downloadDir := t.TempDir()
	options := embeddedBrowserOptions{
		ProfileDir: profileDir,
		AdditionalBrowserArgs: []string{
			"--disable-crash-reporter",
			"--disable-session-crashed-bubble",
			"--disable-sync",
			"--no-first-run",
			"--no-default-browser-check",
			"--disable-blink-features=AutomationControlled",
			"--remote-debugging-address=127.0.0.1",
			"--remote-debugging-port=" + strconv.Itoa(port),
		},
	}

	commands := make(chan embeddedSurfaceCommand)
	stop := make(chan struct{})
	done := make(chan struct{})
	ready := make(chan embeddedSurfaceStartResult, 1)
	go runEmbeddedSurfaceThread(hwnd, port, options, commands, stop, done, ready)
	defer func() {
		close(stop)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}()

	started := <-ready
	if started.err != nil {
		t.Fatalf("create WebView2 controller: %v", started.err)
	}
	rootTargetID, err := waitForDevToolsTarget(
		started.endpoint,
		"about:blank#mhcode-browser-bootstrap-"+strconv.Itoa(port),
		8*time.Second,
	)
	if err != nil {
		t.Fatalf("discover WebView2 bootstrap target: %v", err)
	}

	allocatorCtx, allocatorCancel := chromedp.NewRemoteAllocator(context.Background(), started.endpoint)
	defer allocatorCancel()
	rootCtx, rootCancel := chromedp.NewContext(
		allocatorCtx,
		chromedp.WithTargetID(rootTargetID),
	)
	defer rootCancel()
	if err := chromedp.Run(rootCtx,
		network.Enable(),
		page.Enable(),
		target.SetDiscoverTargets(true),
		browser.SetDownloadBehavior(browser.SetDownloadBehaviorBehaviorAllow).
			WithDownloadPath(downloadDir).
			WithEventsEnabled(true),
	); err != nil {
		t.Fatalf("WebView2 chromedp initialization: %v", err)
	}
}

func removeWebView2TestProfile(t *testing.T, profileDir string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for {
		if err := os.RemoveAll(profileDir); err == nil {
			return
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			t.Errorf("remove WebView2 integration profile: %v", lastErr)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func startWebView2TestWindow(t *testing.T) (uintptr, func()) {
	t.Helper()
	type result struct {
		hwnd uintptr
		err  error
	}
	ready := make(chan result, 1)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)

		className, _ := windows.UTF16PtrFromString("STATIC")
		windowName, _ := windows.UTF16PtrFromString("MHcode WebView2 integration test")
		hwnd, _, callErr := testCreateWindowEx.Call(
			0,
			uintptr(unsafe.Pointer(className)),
			uintptr(unsafe.Pointer(windowName)),
			0x00CF0000,
			0,
			0,
			800,
			600,
			0,
			0,
			0,
			0,
		)
		if hwnd == 0 {
			ready <- result{err: callErr}
			return
		}
		ready <- result{hwnd: hwnd}
		ticker := time.NewTicker(8 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				pumpWindowMessages()
			case <-stop:
				testDestroyWindow.Call(hwnd)
				pumpWindowMessages()
				return
			}
		}
	}()
	started := <-ready
	if started.err != nil || started.hwnd == 0 {
		t.Fatalf("create WebView2 test window: %v", started.err)
	}
	return started.hwnd, func() {
		close(stop)
		<-done
	}
}
