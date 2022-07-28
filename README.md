# GioWebview

[![Go Reference](https://pkg.go.dev/badge/github.com/inkeliz/giowebview.svg)](https://pkg.go.dev/github.com/inkeliz/giowebview)

Give some Webview to your Gio and Golang application. 😎

This project was designed to work with Gio, but it's also possible to use without Gio, providing any HWND (Windows), NSView (macOS), UIView (iOS), HTMLElement (WebAssembly), ViewGroup (Android). Because of that, exists two packages: "GioWebview" and "Webview".

**Currently, GioWebview doesn't work with Gio out-of-box and requires some patches to work.**

> This project is still experimental, so please report any issues you find. 😊

------------------

## How to use it with Gio?

1. **Hook into the Event Loop**: That is necessary to send operations and receive events.

    ```diff
    for evt := range w.Events() { // Gio main event loop
    +    giowebview.Plugin(w, evt)
    
        switch evt := evt.(type) {
            // ...
        }
   }
    ```

2. **Initialize your Webview:** Similar to `widget.Clickable` (and others stateful widgets) you need to create the handler. The handler can be created using `giowebview.NewWebViewOp()`, the handler must be re-used between frames.

    ```go
    if myWebview == nil {
        myWebview = giowebview.NewWebViewOp()
        
       stack := myWebview.Push(gtx.Ops)
      giowebview.NavigateOp{URL: "https://gioui.org"}.Add(gtx.Ops)
      stack.Pop(gtx.Ops)
    }
    ```
3. **Display your WebView:** Similar to `paint.PaintOp` or `clip.RRect` you can set the area which the WebView will show. *The position is absolute and doesn't consider other clips, offsets or transformations*. You can use `giowebview.RectOp{Size: size}.Add(gtx.Ops)` to display the content, and use `giowebview.OffsetOp` to offset.

    ```go
   defer w.WebView.Push(gtx.Ops).Pop(gtx.Ops)
   
   giowebview.OffsetOp{Point: f32.Point{X: 100, Y: 100}}.Add(gtx.Ops)
   giowebview.RectOp{Size: f32.Point{X: 500, Y: 500}}.Add(gtx.Ops)
    ```

There are more features such as read/write cookies, session storage and local storage. Execute javascript and get callbacks from javascript to Golang.

--------------

## Features

We are capable of more than just displaying one webpage.

| OS | Windows | Android | MacOS | iOS | WebAssembly |
| -- | -- | -- | -- | -- | -- |
| Basic Support |✔|✔|✔|✔|✔|
| Setup: Custom Proxy |✔***|✔***|❌|❌|❌|
| Setup: Custom Certificate |✔***|✔***|❌|❌|❌|
| Cookies: Read |✔|✔|✔*|✔|❌|
| Cookies: Write |✔|✔|✔*|✔|❌|
| Cookies: Delete |✔|✔|❌*|✔|❌|
| LocalStorage: Read |✔|✔|✔|✔|❌|
| LocalStorage: Write |✔|✔|✔|✔|❌|
| LocalStorage: Delete |✔|✔|✔|✔|❌|
| SessionStorage: Write |✔|✔|✔|✔|❌|
| SessionStorage: Read |✔|✔|✔|✔|❌|
| SessionStorage: Delete |✔|✔|✔|✔|❌|
| Javascript: Execute |✔|✔|✔|✔|❌|
| Javascript: Install |✔|✔|✔|✔|❌|
| Javascript: Callback |✔**|✔**|✔**|✔**|❌|
| Events: NavigationChange |✔|✔|✔|✔|❌|
| Events: TitleChange |✔|✔|✔|✔|❌|

- ❌ = Not supported.
- ✔ = Supported.

- \* = Cookies can be shared across multiple instances of the WebView. Information from the cookie can be incomplete and lack metadata.
- ** = Only accepts a string as argument (other types are not supported and might be encoded as text).
- *** = Must be defined before the WebView is created and is shared with all instances.

# APIs

Each operating system has uniqueAPI. For Windows 10+, we use WebView2. For Android 6+, we use WebView. For MacOS and
iOS, we use WKWebView. For WebAssembly, the HTMLIFrameElement is used.

# Requirements

- Windows:
    - End-Users: must have Windows 7+ and WebView2 installed (you can install it on the user's machine using the `installview` package).
    - Developers: must have Golang 1.18+ installed (no CGO required).
- WebAssembly:
    - End-Users: must have WebAssembly enabled browser (usually Safari 13+, Chrome 70+).
    - Developers: must have Golang 1.18+ installed (no CGO required).
    - Contributors: must have InkWasm installed.
- macOS:
    - End-Users: must have macOS 11+.
    - Developers: must have macOS device with Golang, Xcode, and CLang installed.
- iOS:
    - End-Users: must have macOS 11+.
    - Developers: must have macOS device with Golang, Xcode, and CLang installed.
- Android:
    - End-Users: must have Android 6 or later.
    - Developers: must have Golang 1.18+, OpenJDK 1.8, Android NDK, Android SDK 31+ installed ([here for more information](https://gioui.org/doc/install/android)).
    - Contributors: must have Android SDK 30 installed.

# Limitations

1. Currently, GioWebview is always the top-most view/window and can't be overlapped by any other draw operation in Gio.
2. Render multiple webviews at the same time might cause unexpected behaviour, related to z-indexes.
3. On Javascript/WebAssembly, it needs to be allowed to iframe the content, which most websites blocks such operation.
4. It's not possible to use WebView using custom shapes (e.g. rounded corners) or apply transformations (e.g. rotating).

# Security

This project uses unsafe CGO for some OSes and shares pointers between Go, Javascript, and other languages, which may expose the application. Some mitigations are possible but not fully implemented, and functional exploits are unknown. Furthermore, GioWebview uses the WebView installed in the system, which can be untrusted or out-of-date.

# License

The source code is licensed under the MIT license.
The pre-compiled DLLs (such as `/webview/sys_windows_386.dll`, `/webview/sys_windows_amd64.dll` and `/webview/sys_windows_arm64.dll`) is redistributed under the BSD license. See LICENSE.md for more information.
