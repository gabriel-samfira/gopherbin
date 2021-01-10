// +build webui

package webui

import (
	"embed"
)

//go:embed web/build
var webUI embed.FS
