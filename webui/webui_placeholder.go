//go:build !webui
// +build !webui

package webui

import (
	"embed"
)

//go:embed placeholder/*
var webUI embed.FS
