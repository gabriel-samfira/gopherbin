//go:build webui
// +build webui

package webui

import (
	"embed"
)

//go:embed all:svelte-app/build
var webUI embed.FS
