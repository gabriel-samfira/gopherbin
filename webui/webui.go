package webui

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const webDir = "svelte-app/build"

// htmlCSP is the Content-Security-Policy served with HTML documents. The
// SvelteKit bootstrap script is inlined by adapter-static, so exact-content
// hashes are added for it; 'unsafe-inline' trails the hashes purely as a
// fallback for pre-CSP3 browsers (CSP3-compliant browsers ignore it as soon
// as a hash or nonce is present).
var htmlCSP = buildHTMLCSP()

// buildHTMLCSP hashes every inline <script> body (tags without attributes)
// of the embedded index.html and embeds the resulting sha256 tokens in the
// policy. Without an embedded bundle (build without the webui tag) it falls
// back to script-src 'self'.
func buildHTMLCSP() string {
	sources := "'self' 'unsafe-inline'"
	contents, err := webUI.ReadFile(path.Join(webDir, "index.html"))
	if err == nil {
		lower := bytes.ToLower(contents)
		var tokens []string
		for i := 0; i < len(contents); {
			j := bytes.Index(lower[i:], []byte("<script>"))
			if j < 0 {
				break
			}
			start := i + j + len("<script>")
			k := bytes.Index(lower[start:], []byte("</script>"))
			if k < 0 {
				break
			}
			sum := sha256.Sum256(contents[start : start+k])
			tokens = append(tokens, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
			i = start + k
		}
		if len(tokens) > 0 {
			sources = "'self' " + strings.Join(tokens, " ")
		}
	}
	return "default-src 'self'; script-src " + sources +
		"; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:;" +
		" connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"
}

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
	if mime == "text/html" {
		// Replace the baseline CSP with the hashed-bootstrap variant.
		w.Header().Set("Content-Security-Policy", htmlCSP)
	}
	w.Header().Set("Last-Modified", buildTime.UTC().Format(http.TimeFormat))
	w.Header().Set("Cache-Control", "max-age=604800")
	w.Write(contents)
}
