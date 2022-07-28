package giowebview

import (
	"gioui.org/app"
	"github.com/inkeliz/giowebview/webview"
)

// NewConfigFromViewEvent creates a webview.Config based on app.ViewEvent.
func NewConfigFromViewEvent(w *app.Window, evt app.ViewEvent) webview.Config {
	return webview.Config{HWND: evt.HWND, RunOnMain: w.Run}
}
