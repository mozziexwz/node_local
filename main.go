// Command MSBOOST-Local is MSBOOST 本地版 (Wails v2). It deploys and manages
// MSBOOST nodes on the user's own servers over SSH from this computer, so SSH
// passwords and keys never reach the website.
//
// It is portable: configs\ and local\ live beside the executable (see
// internal/store), and it runs without administrator rights.
package main

import (
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	xwindows "golang.org/x/sys/windows"

	"github.com/mozziexwz/node_local/internal/logx"
	"github.com/mozziexwz/node_local/internal/store"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	layout, err := store.ExecutableLayout()
	if err != nil {
		fatal("无法确定程序位置：" + err.Error())
	}
	if err := layout.Ensure(); err != nil {
		fatal(err.Error() + "\n请把 MSBOOST 本地版放到可写入的文件夹后再运行。")
	}
	log, err := logx.New(layout.Logs, 14)
	if err != nil {
		fatal("无法创建日志：" + err.Error())
	}
	a := NewApp(layout, log)
	// Fixed size like the launcher: 860×560 leaves about an 844×521 client
	// area once Windows adds the title bar and borders.
	err = wails.Run(&options.App{
		Title:            "MSBOOST 本地版",
		Width:            860,
		Height:           560,
		DisableResize:    true,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 250, G: 249, B: 247, A: 1},
		OnStartup:        a.startup,
		OnBeforeClose:    a.beforeClose,
		OnShutdown:       a.shutdown,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "msboost-local-5c2e91d4",
			OnSecondInstanceLaunch: a.onSecondInstance,
		},
		Windows: &windows.Options{
			WebviewUserDataPath: layout.Webview,
		},
		Bind: []interface{}{a},
	})
	if err != nil {
		fatal("界面启动失败：" + err.Error())
	}
}

// fatal reports a startup failure before any window exists.
func fatal(message string) {
	title, _ := xwindows.UTF16PtrFromString("MSBOOST 本地版")
	text, _ := xwindows.UTF16PtrFromString(message)
	xwindows.MessageBox(0, text, title, xwindows.MB_OK|xwindows.MB_ICONERROR)
	os.Exit(1)
}
