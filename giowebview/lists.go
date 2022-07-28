package giowebview

import (
	"sync"
	"unsafe"

	"gioui.org/app"
	"github.com/inkeliz/giowebview/webview"
)

var _WebViewList sync.Map // map[uintptr]*webview.WebView

func getWebView(t uintptr) (webview.WebView, bool) {
	v, ok := _WebViewList.Load(t)
	if !ok {
		return nil, ok
	}
	return v.(webview.WebView), ok
}

func setWebView(t uintptr, v webview.WebView) {
	_WebViewList.Store(t, v)
}

func removeWebView(t *int64) {
	_WebViewList.Delete(uintptr(unsafe.Pointer(t)))
}

var _PluginList sync.Map // map[*app.Window]*plugin

func getPlugin(t *app.Window) (*plugin, bool) {
	v, ok := _PluginList.Load(uintptr(unsafe.Pointer(t)))
	if !ok {
		return nil, ok
	}
	return v.(*plugin), ok
}

func setPlugin(t *app.Window, v *plugin) {
	_PluginList.Store(uintptr(unsafe.Pointer(t)), v)
}

func removePlugin(t *app.Window) {
	_PluginList.Delete(uintptr(unsafe.Pointer(t)))
}
