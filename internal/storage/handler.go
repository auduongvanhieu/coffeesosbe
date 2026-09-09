package storage

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"coffeesos/internal/httpx"
	"coffeesos/internal/tenant"
)

// MaxUploadBytes caps a single upload. nginx enforces the same limit in
// front of the API (client_max_body_size).
const MaxUploadBytes = 10 << 20 // 10 MiB

type Handler struct{ client *Client }

func NewHandler(c *Client) *Handler { return &Handler{client: c} }

// Upload handles POST /admin/uploads (multipart/form-data).
//
//	file  the image (jpeg, png, webp, gif; svg for logos)
//	kind  menu-items | brand-logos
//
// Responds {key, url}; the caller stores url on the item/brand.
func (h *Handler) Upload(c *gin.Context) {
	brandID, ok := tenant.BrandFrom(c)
	if !ok {
		return
	}
	if !h.client.Enabled() {
		httpx.Fail(c, http.StatusServiceUnavailable, "storage_not_configured", "file storage is not configured on this server")
		return
	}
	kind, ok := ParseKind(c.PostForm("kind"))
	if !ok {
		httpx.Fail(c, http.StatusBadRequest, "invalid_kind", "kind must be menu-items or brand-logos")
		return
	}
	// Logos are brand identity; only owners may change them.
	if kind == KindBrandLogo && tenant.MustFrom(c).Level < tenant.LevelBrandOwner {
		httpx.Fail(c, http.StatusForbidden, "forbidden", "only brand owners can upload logos")
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxUploadBytes+64<<10)
	fh, err := c.FormFile("file")
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			httpx.Fail(c, http.StatusRequestEntityTooLarge, "too_large", "file must be at most 10 MB")
			return
		}
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", "multipart field \"file\" is required")
		return
	}
	if fh.Size > MaxUploadBytes {
		httpx.Fail(c, http.StatusRequestEntityTooLarge, "too_large", "file must be at most 10 MB")
		return
	}

	f, err := fh.Open()
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", "cannot read uploaded file")
		return
	}
	defer f.Close()

	// Trust bytes, not the client-declared type.
	contentType, err := sniff(f)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", "cannot read uploaded file")
		return
	}
	if _, ok := AllowedType(kind, contentType); !ok {
		httpx.Fail(c, http.StatusUnsupportedMediaType, "unsupported_type", "only JPEG, PNG, WebP or GIF images are accepted (SVG for logos)")
		return
	}

	obj, err := h.client.Put(c.Request.Context(), brandID, kind, contentType, f, fh.Size)
	if err != nil {
		_ = c.Error(err)
		httpx.Fail(c, http.StatusBadGateway, "storage_error", "could not store the file")
		return
	}
	c.JSON(http.StatusCreated, obj)
}
