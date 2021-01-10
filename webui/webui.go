package webui

import (
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"time"
)

const webDir = "web/build"

// BuildTime can be set to enable Last-Modified and
// If-Modified-Since functionality in the web UI handler.
var BuildTime string

var buildTime time.Time

func init() {
	if BuildTime != "" {
		asInt, err := strconv.ParseInt(BuildTime, 10, 64)
		if err != nil {
			return
		}

		buildTime = time.Unix(asInt, 0)
	} else {
		now := time.Now()
		buildTime = now.Add(time.Hour * -48)
	}
}

func determineMIMEType(filename string, content []byte) string {
	switch filepath.Ext(filename) {
	case ".js":
		return "text/javascript"
	case ".html", ".htm":
		return "text/html"
	case ".ico":
		return "image/vnd.microsoft.icon"
	case ".png":
		return "image/png"
	case ".svg":
		return "image/svg+xml"
	case ".css":
		return "text/css"
	default:
		return http.DetectContentType(content)
	}
}

// UIHandler returns the web UI
func UIHandler(w http.ResponseWriter, r *http.Request) {
	requestedFile := r.URL.Path[1:]

	if requestedFile == "" {
		requestedFile = "index.html"
	}

	fullPath := path.Join(webDir, requestedFile)

	contents, err := webUI.ReadFile(fullPath)
	if err != nil {
		fullPath = path.Join(webDir, "index.html")
		contents, err = webUI.ReadFile(fullPath)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Page not found"))
			return
		}
	}

	mime := determineMIMEType(fullPath, contents)
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Last-Modified", buildTime.UTC().Format(http.TimeFormat))
	w.Write(contents)
}
