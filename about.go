package main

import (
	_ "embed"
	"encoding/json"
	"runtime/debug"
	"strings"

	"github.com/mozziexwz/node_local/installers"
)

// version is set at build time with -ldflags "-X main.version=v1.2.3".
// Wails builds with -buildvcs=false, so CI passes the commit this way too.
var version = "dev"

const copyrightNotice = "Copyright © 2026 MSBOOST"

//go:embed UPSTREAM.json
var upstreamJSON []byte

// versionString is the release version plus the source revision when known.
func versionString() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}
	rev, dirty := "", ""
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) >= 7 {
				rev = s.Value[:7]
			}
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "+"
			}
		}
	}
	if rev == "" {
		return version
	}
	return version + " (" + rev + dirty + ")"
}

func moduleVersion(path string) string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, dep := range info.Deps {
		if dep.Path == path {
			if dep.Replace != nil {
				return dep.Replace.Version
			}
			return dep.Version
		}
	}
	return ""
}

// scriptsSource says which website release the bundled installer comes from.
func scriptsSource() string {
	var m struct{ Tag, Commit string }
	if json.Unmarshal(upstreamJSON, &m) != nil || len(m.Commit) < 7 {
		return ""
	}
	return "安装脚本同步自 MSBOOST 网站 " + m.Tag + "（" + m.Commit[:7] + "）"
}

// sourceNotice says where the source of a release is offered: next to the
// program, wherever it is downloaded (GPL-3.0 section 6d). The website does
// not link the repository. Test builds are not distributed, so they say
// nothing.
func sourceNotice() string {
	if !strings.HasPrefix(version, "v") {
		return ""
	}
	return "同版本的完整源代码与本程序在同一下载位置提供：MSBOOST-Local-" + version + "-source.zip。"
}

// aboutInfo backs the about card. GPL-3.0 requires interactive programs to
// show a copyright notice, the absence of warranty and how to read the
// license. Component names appear only in the license dialog.
func aboutInfo() AboutDTO {
	return AboutDTO{
		Version:   versionString(),
		Copyright: copyrightNotice,
		License: "MSBOOST 本地版是自由软件：你可以依据自由软件基金会发布的 GNU 通用公共许可证第 3 版（GPL-3.0）" +
			"再发布和修改本程序。本程序不提供任何担保。本程序只连接你自己的服务器，不连接 MSBOOST 网站，也不上传任何信息。",
		Source:  sourceNotice(),
		Scripts: scriptsSource(),
		Components: []ComponentDTO{
			{Name: "Wails", Version: moduleVersion("github.com/wailsapp/wails/v2"), Copyright: "Copyright (c) 2018-Present Lea Anthony",
				License: "MIT", URL: "https://wails.io"},
			{Name: "Go 扩展库（crypto、net、sys）", Version: moduleVersion("golang.org/x/crypto"), Copyright: "Copyright 2009 The Go Authors",
				License: "BSD-3-Clause", URL: "https://pkg.go.dev/golang.org/x/crypto"},
			{Name: "React", Version: "19", Copyright: "Copyright (c) Meta Platforms, Inc. and affiliates.",
				License: "MIT", URL: "https://react.dev"},
		},
		// Downloaded by the server from their GitHub releases during a deploy
		// or relay setup; this program does not contain them.
		ServerSide: []ComponentDTO{
			{Name: "mihomo（节点引擎）", Version: installers.EngineVersion(), Copyright: "Clash 与 MetaCubeX 贡献者",
				License: "GPL-3.0", URL: "https://github.com/MetaCubeX/mihomo"},
			{Name: "GOST（中转）", Version: installers.GostVersion(), Copyright: "Copyright (c) 2016 ginuerzh",
				License: "MIT", URL: "https://github.com/go-gost/gost"},
		},
		LicenseTexts: []LinkDTO{
			{Name: "GPL-3.0", URL: "https://www.gnu.org/licenses/gpl-3.0.html"},
			{Name: "MIT", URL: "https://opensource.org/license/mit"},
			{Name: "BSD-3-Clause", URL: "https://opensource.org/license/bsd-3-clause"},
		},
	}
}
