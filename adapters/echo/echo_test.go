package chanxecho_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	chanx "github.com/huynguyengl99/chanx-go"
	chanxecho "github.com/huynguyengl99/chanx-go/adapters/echo"
	"github.com/huynguyengl99/chanx-go/asyncapi"
	"github.com/huynguyengl99/chanx-go/chanxtest/adaptertest"
)

func TestAdapter(t *testing.T) {
	adaptertest.Run(t, func(t *testing.T, e *chanx.Engine) *adaptertest.Server {
		app := echo.New()
		auth := func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c *echo.Context) error {
				if c.Request().Header.Get("X-Reject") != "" {
					return echo.NewHTTPError(http.StatusUnauthorized, "rejected")
				}
				if u := c.Request().Header.Get("X-User"); u != "" {
					c.Set("user", u)
				}
				return next(c)
			}
		}
		chanxecho.Mount(app.Group("", auth), e)
		srv := httptest.NewServer(app)
		t.Cleanup(srv.Close)
		return &adaptertest.Server{URL: srv.URL}
	})
}

func TestMountDocs(t *testing.T) {
	e := chanx.New(chanx.Config{})
	r := echo.New()
	chanxecho.MountDocs(r, e, "/asyncapi", asyncapi.Info{Title: "Docs"})
	for _, p := range []string{"/asyncapi", "/asyncapi.json", "/asyncapi.yaml"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Docs") {
			t.Errorf("%s: %d", p, rec.Code)
		}
	}
}
