package web

import (
	"context"
	"encoding/base64"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/preview"
	"github.com/bjschafer/print-dis/internal/store"
)

// Previewer reads a link's title and thumbnail. *preview.Fetcher is the
// real one; tests substitute a fake.
type Previewer interface {
	Fetch(ctx context.Context, rawURL string) (*preview.Preview, error)
}

const previewTimeout = 10 * time.Second

const previewFailed = "Couldn't read that page; add a title yourself."

var thumbTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true, "image/gif": true}

// prefillFromLink fills the new-request form from req.SourceURL. Failure is
// normal (many sites block servers), so it only adds a note.
//
// The thumbnail is shown inline but is not sent back to the server: the form
// carries only a fetch_image=1 flag and createRequest downloads the image
// again, through the same SSRF guard, so the browser never supplies image
// bytes that get stored.
func (s *Server) prefillFromLink(r *http.Request, req *core.Request, v *requestFormView) {
	if s.Previewer == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), previewTimeout)
	defer cancel()
	p, err := s.Previewer.Fetch(ctx, *req.SourceURL)
	if err != nil || p == nil {
		s.Log.Info("link preview failed", "err", err)
		v.PreviewNote = previewFailed
		return
	}
	if p.Title != "" {
		req.Title = p.Title
		t := p.Title
		req.SourceTitle = &t
	}
	if len(p.Image) > 0 && thumbTypes[p.ImageType] {
		v.Thumb = template.URL("data:" + p.ImageType + ";base64," + base64.StdEncoding.EncodeToString(p.Image)) // #nosec G203 -- type is from a fixed allowlist, payload is base64
		v.FetchImage = true
	}
	if p.Title == "" {
		v.PreviewNote = previewFailed
	}
}

// attachThumbnail downloads the thumbnail for req.SourceURL. No thumbnail is
// not an error.
func (s *Server) attachThumbnail(r *http.Request, req *core.Request) {
	if s.Previewer == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), previewTimeout)
	defer cancel()
	p, err := s.Previewer.Fetch(ctx, *req.SourceURL)
	if err != nil || p == nil || len(p.Image) == 0 || len(p.Image) > preview.MaxImageBytes || !thumbTypes[p.ImageType] {
		return
	}
	req.SourceImage = p.Image
	typ := p.ImageType
	req.SourceImageType = &typ
}

type helpView struct {
	base
	Bookmarklet template.URL
	BaseURL     string
	// The API panel, for admins: their tokens, and one just created, which
	// is shown this once.
	Tokens   []store.APIToken
	NewToken string
}

func (s *Server) help(w http.ResponseWriter, r *http.Request) {
	base := strings.TrimRight(s.Config.BaseURL.String(), "/")
	v := helpView{base: s.base(r, "Help"), Bookmarklet: bookmarklet(base), BaseURL: base}
	if core.CanUseAPI(v.User) {
		var err error
		if v.Tokens, err = s.Store.ListAPITokens(r.Context(), v.User.ID); err != nil {
			s.serverError(w, r, err)
			return
		}
		v.NewToken = s.Auth.Sessions.PopString(r.Context(), newTokenKey)
	}
	s.render(w, r, http.StatusOK, "help.html", v)
}

// bookmarklet builds the javascript: URL from the configured base URL only.
func bookmarklet(base string) template.URL {
	lit := strings.NewReplacer(`\`, `\\`, `'`, `\'`, "<", `\x3c`, "\n", "").Replace(base)
	js := "javascript:(function(){window.open('" + lit + "/requests/new?url='+encodeURIComponent(location.href),'_blank');})();"
	return template.URL(js) // #nosec G203 -- built from Config.BaseURL, never from request data
}
