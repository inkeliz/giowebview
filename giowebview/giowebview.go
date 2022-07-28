package giowebview

import (
	"image"
	"net/url"
	"runtime"
	"sync"
	"unsafe"

	"gioui.org/unit"
	"github.com/inkeliz/giowebview/webview"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/system"
	"gioui.org/op"
)

type plugin struct {
	queue *queue

	seemMutex sync.Mutex
	seem      map[webview.WebView]bool
	bounds    map[webview.WebView]*[2]f32.Point

	active webview.WebView

	config    webview.Config
	viewEvent app.ViewEvent
}

type queue struct {
	event.Queue
	events map[event.Tag][]event.Event
}

// Events returns events for the given event.Tag.
// It will return Gio events and WebView events.
func (q *queue) Events(t event.Tag) []event.Event {
	if w, ok := t.(*WebViewOp); ok {
		if w != nil {
			t = w.tag
		}
	}
	if evts, ok := q.events[t]; ok || len(evts) > 0 {
		q.events[t] = append(q.events[t], q.Queue.Events(t)...)
		r := evts
		q.events[t] = evts[:0]
		return r
	}
	return q.Queue.Events(t)
}

func (q *queue) add(t event.Tag, evt event.Event) {
	q.events[t] = append(q.events[t], evt)
}

// gioInternalOps must match with gioui.org/op/ops
// That is internal, we use unsafe. >:)
type gioInternalOps struct {
	version int
	data    []byte
	refs    []interface{}
}

func (ops *gioInternalOps) write(v interface{}) {
	ops.refs = append(ops.refs, v)
}

// Plugin hijacks Gio events and wraps them in WebView events.
// You must call at the beginning of your window.Event function, like this:
//
//		for evt := range w.Events() {
//			giowebview.Plugin(w, evt)
//
// 		    switch evt := evt.(type) {
// 		    // ...
// 		    }
//		}
//
func Plugin(w *app.Window, evt event.Event) {
	p, ok := getPlugin(w)
	if !ok {
		p = &plugin{
			queue:  &queue{events: make(map[event.Tag][]event.Event)},
			seem:   make(map[webview.WebView]bool, 8),
			bounds: make(map[webview.WebView]*[2]f32.Point, 8),
		}
		setPlugin(w, p)
	}

	switch e := evt.(type) {
	case app.ViewEvent:
		p.viewEvent = e
		config := NewConfigFromViewEvent(w, e)
		for v := range p.seem {
			if v == nil {
				continue
			}
			v.Configure(config)
		}
		p.config = config
	case system.DestroyEvent:
		p.seemMutex.Lock()
		for s := range p.seem {
			s.Close()
			delete(p.seem, s)
		}
		p.seemMutex.Unlock()
	case system.FrameEvent:
		ref := *(**system.FrameEvent)(unsafe.Add(unsafe.Pointer(&evt), unsafe.Sizeof(uintptr(0))))

		p.queue.Queue = ref.Queue
		ref.Queue = p.queue

		fn := ref.Frame
		ref.Frame = func(frame *op.Ops) {
			ops := (*gioInternalOps)(unsafe.Pointer(&frame.Internal))
			fn(frame)
			for s := range p.seem {
				p.seem[s] = false
			}

			for i := range ops.refs {
				switch v := ops.refs[i].(type) {
				case *RectOp:
					p.seem[p.active] = true
					v.execute(w, p, e)
				case interface {
					execute(w *app.Window, p *plugin, _ system.FrameEvent)
				}:
					v.execute(w, p, e)
				default:
					_ = v
				}
			}

			for k, v := range p.seem {
				if k != nil && !v {
					k.Resize(webview.Point{}, webview.Point{})
				}
			}

			for _, v := range p.bounds {
				v[0] = f32.Point{}
				v[1] = f32.Point{}
			}

			p.active = nil

		}
	}
}

// WebViewOp shows the webview into the specified area.
// The RectOp is not context-aware, and will overlay
// any other widget on the screen.
//
// WebViewOp also takes the foreground and clicks events
// and keyboard events will not be routed to Gio.
//
// Performance: changing the size/bounds or radius can
// be expensive. If applicable, change the Offset, instead
// of changing the size.
type WebViewOp struct {
	wvTag *int64
	tag   event.Tag
	isPop bool
}

type webViewOp struct {
	wvTag uintptr // *int64 (avoid GC track)
	tag   event.Tag
	isPop bool
}

// NewWebViewOp creates a new WebViewOp.
func NewWebViewOp() *WebViewOp {
	r := &WebViewOp{wvTag: new(int64)}
	runtime.SetFinalizer(r, func(r *WebViewOp) {
		if v, ok := getWebView(uintptr(unsafe.Pointer(r.wvTag))); ok {
			v.Close()
			_PluginList.Range(func(_, p interface{}) bool {
				p.(*plugin).seemMutex.Lock()
				defer p.(*plugin).seemMutex.Unlock()

				delete(p.(*plugin).seem, v)
				return true
			})
			removeWebView(r.wvTag)
		}
	})

	r.tag = event.Tag(uintptr(unsafe.Pointer(r)))
	return r
}

var poolWebViewOp = newPool[webViewOp](func() any { return new(webViewOp) })

// Push adds a new WebViewOp to the queue, any subsequent Ops (sucha as RectOp)
// will affect this WebViewOp.
// In order to stop using this WebViewOp, call Pop.
func (o WebViewOp) Push(op *op.Ops) WebViewOp {
	o.isPop = false
	poolWebViewOp.add(op, *(*webViewOp)(unsafe.Pointer(&o)))
	return o
}

// Pop stops using the WebViewOp.
func (o WebViewOp) Pop(op *op.Ops) {
	o.isPop = true
	poolWebViewOp.add(op, *(*webViewOp)(unsafe.Pointer(&o)))
}

func (o *webViewOp) execute(w *app.Window, p *plugin, e system.FrameEvent) {
	defer poolWebViewOp.free(o)

	if o.isPop {
		p.active = nil
		return
	}
	p.seemMutex.Lock()
	defer p.seemMutex.Unlock()

	runner, ok := getWebView(o.wvTag)
	if !ok {
		wv, err := webview.NewWebView(NewConfigFromViewEvent(w, p.viewEvent))
		if err != nil {
			panic(err)
		}
		go eventsListener(wv, w, p, o.tag)
		runner = wv
		setWebView(o.wvTag, runner)
	}

	if _, ok := p.seem[runner]; !ok {
		p.seem[runner] = false
	}

	p.active = runner
}

func eventsListener(wv webview.WebView, w *app.Window, p *plugin, tag event.Tag) {
	for evt := range wv.Events() {
		switch evt := evt.(type) {
		case webview.NavigationEvent:
			p.queue.add(tag, NavigationEvent(evt))
		case webview.TitleEvent:
			p.queue.add(tag, TitleEvent(evt))
		}
		w.Invalidate()
	}
}

// OffsetOp moves the webview by the specified offset.
type OffsetOp struct {
	Point f32.Point
}

// Offset creates a new OffsetOp.
func Offset[POINT image.Point | f32.Point](v POINT) OffsetOp {
	switch v := any(v).(type) {
	case image.Point:
		return OffsetOp{Point: f32.Point{X: float32(v.X), Y: float32(v.Y)}}
	case f32.Point:
		return OffsetOp{Point: v}
	default:
		return OffsetOp{}
	}
}

var poolOffsetOp = newPool[OffsetOp](func() any { return new(OffsetOp) })

// Add adds a new OffsetOp to the queue.
func (o OffsetOp) Add(op *op.Ops) {
	poolOffsetOp.add(op, o)
}

func (o *OffsetOp) execute(w *app.Window, p *plugin, e system.FrameEvent) {
	defer poolOffsetOp.free(o)
	if _, ok := p.bounds[p.active]; !ok {
		p.bounds[p.active] = new([2]f32.Point)
	}
	p.bounds[p.active][0].Y += o.Point.Y
	p.bounds[p.active][0].X += o.Point.X
}

// RectOp shows the webview into the specified area.
// The RectOp is not context-aware, and will overlay
// any other widget on the screen.
//
// RectOp also takes the foreground and clicks events
// and keyboard events will not be routed to Gio.
//
// Performance: changing the size/bounds or radius can
// be expensive. If applicable, change the Rect, instead
// of changing the size.
//
// Only one RectOp can be active at each frame for the
// same WebViewOp.
type RectOp struct {
	Size           f32.Point
	SE, SW, NW, NE float32
}

// Rect creates a new RectOp.
func Rect[POINT image.Point | f32.Point](v POINT) RectOp {
	switch v := any(v).(type) {
	case image.Point:
		return RectOp{Size: f32.Point{X: float32(v.X), Y: float32(v.Y)}}
	case f32.Point:
		return RectOp{Size: v}
	default:
		return RectOp{}
	}
}

var poolRectOp = newPool[RectOp](func() any { return new(RectOp) })

// Add adds a new RectOp to the queue.
func (o RectOp) Add(op *op.Ops) {
	poolRectOp.add(op, o)
}

func (o *RectOp) execute(w *app.Window, p *plugin, e system.FrameEvent) {
	defer poolRectOp.free(o)
	p.seemMutex.Lock()
	defer p.seemMutex.Unlock()

	p.bounds[p.active][1].X += o.Size.X
	p.bounds[p.active][1].Y += o.Size.Y

	if _, ok := p.bounds[p.active]; !ok {
		p.bounds[p.active] = new([2]f32.Point)
	}

	p.bounds[p.active][0].X += float32(unit.Dp(e.Metric.PxPerDp) * e.Insets.Left)
	p.bounds[p.active][0].Y += float32(unit.Dp(e.Metric.PxPerDp) * e.Insets.Top)

	p.active.Resize(
		webview.Point{X: p.bounds[p.active][1].X, Y: p.bounds[p.active][1].Y},
		webview.Point{X: p.bounds[p.active][0].X, Y: p.bounds[p.active][0].Y},
	)
}

// NavigateOp redirects the last Display to the
// given URL. If the URL have unknown protocols,
// or malformed URL may lead to unknown behaviors.
type NavigateOp struct {
	// URL is the URL to redirect to.
	URL string
}

var poolNavigateOp = newPool[NavigateOp](func() any { return new(NavigateOp) })

// Add adds a new NavigateOp to the queue.
func (o NavigateOp) Add(op *op.Ops) {
	poolNavigateOp.add(op, o)
}

func (o *NavigateOp) execute(w *app.Window, p *plugin, e system.FrameEvent) {
	defer poolNavigateOp.free(o)

	if e.Metric.PxPerDp != p.config.PxPerDp {
		p.config.PxPerDp = e.Metric.PxPerDp
		p.active.Configure(p.config)
	}

	u, err := url.Parse(o.URL)
	if err != nil {
		return
	}
	p.active.Navigate(u)
}

// SetCookieOp sets given cookie in the webview.
type SetCookieOp struct {
	Cookie webview.CookieData
}

var poolSetCookieOp = newPool[SetCookieOp](func() any { return new(SetCookieOp) })

// Add adds a new SetCookieOp to the queue.
func (o SetCookieOp) Add(op *op.Ops) {
	poolSetCookieOp.add(op, o)
}

func (o *SetCookieOp) execute(w *app.Window, p *plugin, _ system.FrameEvent) {
	manager := p.active.DataManager()

	go func() {
		defer poolSetCookieOp.free(o)
		manager.AddCookie(o.Cookie)
	}()
}

// RemoveCookieOp sets given cookie in the webview.
type RemoveCookieOp struct {
	Cookie webview.CookieData
}

var poolRemoveCookieOp = newPool[RemoveCookieOp](func() any { return new(RemoveCookieOp) })

// Add adds a new SetCookieOp to the queue.
func (o RemoveCookieOp) Add(op *op.Ops) {
	poolRemoveCookieOp.add(op, o)
}

func (o *RemoveCookieOp) execute(w *app.Window, p *plugin, _ system.FrameEvent) {
	manager := p.active.DataManager()

	go func() {
		defer poolRemoveCookieOp.free(o)
		manager.RemoveCookie(o.Cookie)
	}()
}

// ListCookieOp lists all cookies in the webview.
// The response in sent via CookiesEvent using the
// provided Tag.
type ListCookieOp struct {
	Tag event.Tag
}

// CookiesEvent is the event sent when ListCookieOp is executed.
type CookiesEvent struct {
	Cookies []webview.CookieData
}

// ImplementsEvent the event.Event interface.
func (c CookiesEvent) ImplementsEvent() {}

var poolListCookieOp = newPool[ListCookieOp](func() any { return new(ListCookieOp) })

// Add adds a new ListCookieOp to the queue.
func (o ListCookieOp) Add(op *op.Ops) {
	poolListCookieOp.add(op, o)
}

func (o *ListCookieOp) execute(w *app.Window, p *plugin, _ system.FrameEvent) {
	manager := p.active.DataManager()

	go func() {
		defer poolListCookieOp.free(o)
		evt := CookiesEvent{}
		manager.Cookies(func(c *webview.CookieData) bool {
			evt.Cookies = append(evt.Cookies, *c)
			return true
		})
		p.queue.add(o.Tag, evt)
	}()
}

// StorageType is the type of storage.
type StorageType int

const (
	// StorageTypeLocal is the local storage.
	StorageTypeLocal StorageType = iota
	// StorageTypeSession is the session storage.
	StorageTypeSession
)

// SetStorageOp sets given Storage in the webview.
type SetStorageOp struct {
	Local   StorageType
	Content webview.StorageData
}

var poolSetStorageOp = newPool[SetStorageOp](func() any { return new(SetStorageOp) })

// Add adds a new SetStorageOp to the queue.
func (o SetStorageOp) Add(op *op.Ops) {
	poolSetStorageOp.add(op, o)
}

func (o *SetStorageOp) execute(w *app.Window, p *plugin, _ system.FrameEvent) {
	manager := p.active.DataManager()

	go func() {
		defer poolSetStorageOp.free(o)
		switch o.Local {
		case StorageTypeLocal:
			manager.AddLocalStorage(o.Content)
		case StorageTypeSession:
			manager.AddSessionStorage(o.Content)
		}
	}()
}

// RemoveStorageOp sets given Storage in the webview.
type RemoveStorageOp struct {
	Local   StorageType
	Content webview.StorageData
}

var poolRemoveStorageOp = newPool[RemoveStorageOp](func() any { return new(RemoveStorageOp) })

// Add adds a new SetStorageOp to the queue.
func (o RemoveStorageOp) Add(op *op.Ops) {
	poolRemoveStorageOp.add(op, o)
}

func (o *RemoveStorageOp) execute(w *app.Window, p *plugin, _ system.FrameEvent) {
	manager := p.active.DataManager()

	go func() {
		defer poolRemoveStorageOp.free(o)
		switch o.Local {
		case StorageTypeLocal:
			manager.RemoveLocalStorage(o.Content)
		case StorageTypeSession:
			manager.AddSessionStorage(o.Content)
		}
	}()
}

// ListStorageOp lists all Storage in the webview.
// The response in sent via StorageEvent using the
// provided Tag.
type ListStorageOp struct {
	Local StorageType
	Tag   event.Tag
}

// StorageEvent is the event sent when ListStorageOp is executed.
type StorageEvent struct {
	Storage []webview.StorageData
}

// ImplementsEvent the event.Event interface.
func (c StorageEvent) ImplementsEvent() {}

var poolListStorageOp = newPool[ListStorageOp](func() any { return new(ListStorageOp) })

// Add adds a new ListStorageOp to the queue.
func (o ListStorageOp) Add(op *op.Ops) {
	poolListStorageOp.add(op, o)
}

func (o *ListStorageOp) execute(w *app.Window, p *plugin, _ system.FrameEvent) {
	manager := p.active.DataManager()

	go func() {
		defer poolListStorageOp.free(o)
		evt := StorageEvent{}

		fn := manager.LocalStorage
		if o.Local == StorageTypeSession {
			fn = manager.SessionStorage
		}

		fn(func(c *webview.StorageData) bool {
			evt.Storage = append(evt.Storage, *c)
			return true
		})

		p.queue.add(o.Tag, evt)
	}()
}

// ExecuteJavascriptOp executes given JavaScript in the webview.
type ExecuteJavascriptOp struct {
	Script string
}

var poolExecuteJavascript = newPool[ExecuteJavascriptOp](func() any { return new(ExecuteJavascriptOp) })

// Add adds a new ExecuteJavascriptOp to the queue.
func (o ExecuteJavascriptOp) Add(op *op.Ops) {
	poolExecuteJavascript.add(op, o)
}

func (o *ExecuteJavascriptOp) execute(w *app.Window, p *plugin, _ system.FrameEvent) {
	manager := p.active.JavascriptManager()

	go func() {
		defer poolExecuteJavascript.free(o)
		manager.RunJavaScript(o.Script)
	}()
}

// InstallJavascriptOp installs given JavaScript in the webview, executing
// it every time the webview loads a new page. The script is executed before
// the page is fully loaded.
type InstallJavascriptOp struct {
	Script string
}

var poolInstallJavascript = newPool[InstallJavascriptOp](func() any { return new(InstallJavascriptOp) })

// Add adds a new ExecuteJavascriptOp to the queue.
func (o InstallJavascriptOp) Add(op *op.Ops) {
	poolInstallJavascript.add(op, o)
}

func (o *InstallJavascriptOp) execute(w *app.Window, p *plugin, _ system.FrameEvent) {
	manager := p.active.JavascriptManager()

	go func() {
		defer poolInstallJavascript.free(o)
		manager.InstallJavascript(o.Script, webview.JavascriptOnLoadStart)
	}()
}

// MessageReceiverOp receives a message from the webview,
// and sends it to the provided Tag. The message is sent
// as a string.
//
// You can use this to communicate with the webview, by using:
//     window.callback.<name>(<message>);
//
// Consider that <name> is the provided Name of the callback,
// and <message> is the message to send to Tag.
//
// For further information, see webview.JavascriptManager.
type MessageReceiverOp struct {
	Name string
	Tag  event.Tag
}

// MessageEvent is the event sent when receiving a message,
// from previously defined MessageReceiverOp.
type MessageEvent struct {
	Message string
}

// ImplementsEvent the event.Event interface.
func (c MessageEvent) ImplementsEvent() {}

var poolMessageReceiver = newPool[MessageReceiverOp](func() any { return new(MessageReceiverOp) })

// Add adds a new ExecuteJavascriptOp to the queue.
func (o MessageReceiverOp) Add(op *op.Ops) {
	poolMessageReceiver.add(op, o)
}

func (o *MessageReceiverOp) execute(w *app.Window, p *plugin, _ system.FrameEvent) {
	defer poolMessageReceiver.free(o)

	p.active.JavascriptManager().AddCallback(o.Name, func(msg string) {
		p.queue.add(o.Tag, MessageEvent{Message: msg})
	})
}

// NavigationEvent is issued when the webview change the URL.
type NavigationEvent webview.NavigationEvent

// ImplementsEvent the event.Event interface.
func (NavigationEvent) ImplementsEvent() {}

// TitleEvent is issued when the webview change the title.
type TitleEvent webview.TitleEvent

// ImplementsEvent the event.Event interface.
func (TitleEvent) ImplementsEvent() {}
