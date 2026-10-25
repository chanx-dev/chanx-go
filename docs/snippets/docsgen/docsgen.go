// Package docsgen holds the examples of the AsyncAPI guide.
package docsgen

import (
	"flag"
	"net/http"
	"os"

	chanx "github.com/huynguyengl99/chanx-go"
	"github.com/huynguyengl99/chanx-go/asyncapi"
	chanxhttp "github.com/huynguyengl99/chanx-go/nethttp"
)

// #region info
var Info = asyncapi.Info{
	Title:       "Chat API",
	Version:     "2.1.0",
	Description: "Rooms, presence and notifications.",
	Servers: map[string]any{
		"production": map[string]any{"host": "chat.example.com", "protocol": "wss"},
		"staging":    map[string]any{"host": "staging.chat.example.com", "protocol": "wss"},
	},
}

// #endregion info

// #region mount
func Mount(mux *http.ServeMux, engine *chanx.Engine) {
	chanxhttp.MountDocs(mux, engine, "/asyncapi", Info) // /asyncapi, /asyncapi.json, /asyncapi.yaml
}

// #endregion mount

// #region flag
// MaybeWrite prints the document and exits with -asyncapi, for CI and client
// generators: go run . -asyncapi > asyncapi.json
func MaybeWrite(engine *chanx.Engine) {
	doc := flag.Bool("asyncapi", false, "print the AsyncAPI document and exit")
	flag.Parse()
	if *doc {
		if err := asyncapi.Write(os.Stdout, engine, Info); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
}

// #endregion flag

// #region selfhost
// SelfHosted serves the docs page's script and stylesheet from your own files,
// for a page that loads nothing from a CDN.
var SelfHosted = asyncapi.Info{
	Title:      "Chat API",
	DocsJSURL:  "/static/asyncapi/index.js",
	DocsCSSURL: "/static/asyncapi/default.min.css",
}

// #endregion selfhost
