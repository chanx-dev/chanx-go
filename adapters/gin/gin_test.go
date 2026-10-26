package chanxgin_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	chanxgin "go.chanx.dev/adapters/gin/v2"
	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/asyncapi"
	"go.chanx.dev/v2/chanxtest/adaptertest"
	"go.chanx.dev/v2/transport"
	"go.chanx.dev/v2/transport/coderws"
	"go.chanx.dev/v2/transport/gorillaws"
)

func TestAdapter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, opts := range map[string][]chanxgin.Option{
		"default":        nil,
		"accept options": {chanxgin.AcceptOptions(&coderws.Options{InsecureSkipVerify: true})},
		"gorilla": {chanxgin.Upgrade(func(w http.ResponseWriter, r *http.Request) (transport.Conn, error) {
			return gorillaws.Accept(w, r, nil)
		})},
	} {
		t.Run(name, func(t *testing.T) {
			adaptertest.Run(t, func(t *testing.T, e *chanx.Engine) *adaptertest.Server {
				r := gin.New()
				auth := func(c *gin.Context) {
					if c.GetHeader("X-Reject") != "" {
						c.AbortWithStatus(http.StatusUnauthorized)
						return
					}
					if u := c.GetHeader("X-User"); u != "" {
						c.Set("user", u)
					}
				}
				chanxgin.Mount(r.Group("/", auth), e, opts...)
				srv := httptest.NewServer(r)
				t.Cleanup(srv.Close)
				return &adaptertest.Server{URL: srv.URL}
			})
		})
	}
}

func TestMountDocs(t *testing.T) {
	e := chanx.New(chanx.Config{})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	chanxgin.MountDocs(r, e, "/asyncapi", asyncapi.Info{Title: "Docs"})
	for _, p := range []string{"/asyncapi", "/asyncapi.json", "/asyncapi.yaml"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Docs") {
			t.Errorf("%s: %d", p, rec.Code)
		}
	}
}
