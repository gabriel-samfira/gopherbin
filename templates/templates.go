package templates

import (
	"github.com/gobuffalo/packr/v2"
)

// AssetsBox is a packr box containing js, css, etc
var AssetsBox = packr.New("assets", "./assets")

// TemplateBox is a packr box containing html templates
var TemplateBox = packr.New("templates", "./html")
