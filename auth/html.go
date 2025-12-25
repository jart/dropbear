package auth

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var staticFiles embed.FS

func staticFS() http.FileSystem {
	sub, _ := fs.Sub(staticFiles, "static")
	return http.FS(sub)
}

func serveFile(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := staticFiles.ReadFile("static/" + name)
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		if len(name) > 4 {
			switch name[len(name)-4:] {
			case ".css":
				w.Header().Set("Content-Type", "text/css")
			case "html":
				w.Header().Set("Content-Type", "text/html")
			}
		}
		if len(name) > 3 && name[len(name)-3:] == ".js" {
			w.Header().Set("Content-Type", "application/javascript")
		}
		w.Write(data)
	}
}

// StaticHandler returns a handler for serving auth static files
func StaticHandler() http.Handler {
	return http.FileServer(staticFS())
}
