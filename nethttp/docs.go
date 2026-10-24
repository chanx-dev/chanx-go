package chanxhttp

import (
	chanx "github.com/huynguyengl99/chanx-go"
	"github.com/huynguyengl99/chanx-go/asyncapi"
)

// MountDocs serves the AsyncAPI page at path, the JSON document at path+".json"
// and YAML at path+".yaml".
func MountDocs(mux Router, e *chanx.Engine, path string, info asyncapi.Info) {
	mux.Handle("GET "+path, asyncapi.HTMLHandler(e, info))
	mux.Handle("GET "+path+".json", asyncapi.JSONHandler(e, info))
	mux.Handle("GET "+path+".yaml", asyncapi.YAMLHandler(e, info))
}
