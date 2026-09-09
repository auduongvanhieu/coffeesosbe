// Package storage stores uploaded files (menu images, brand logos) in
// Cloudflare R2 through its S3-compatible API and returns public CDN URLs.
//
// Objects are written under brands/{brandId}/{kind}/{uuid}.{ext}. Keys are
// unique per upload, so objects are served with an immutable cache header and
// replacing an image simply means uploading a new key.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

// ErrNotConfigured is returned when R2 credentials are missing; handlers map
// it to a 503 so the rest of the API keeps working without object storage.
var ErrNotConfigured = errors.New("storage: R2 is not configured")

type Config struct {
	AccountID       string // Cloudflare account id
	AccessKeyID     string // R2 API token (S3 credentials)
	SecretAccessKey string
	Bucket          string
	PublicBaseURL   string // e.g. https://cdn.coffeesos.online (custom domain on the bucket)
}

func (c Config) Enabled() bool {
	return c.AccountID != "" && c.AccessKeyID != "" && c.SecretAccessKey != "" && c.Bucket != "" && c.PublicBaseURL != ""
}

// Kind is the logical folder an upload belongs to.
type Kind string

const (
	KindMenuItem  Kind = "menu-items"
	KindBrandLogo Kind = "brand-logos"
)

func ParseKind(s string) (Kind, bool) {
	switch Kind(s) {
	case KindMenuItem, KindBrandLogo:
		return Kind(s), true
	}
	return "", false
}

// Allowed content types and their canonical file extensions.
var extByType = map[string]string{
	"image/jpeg":    "jpg",
	"image/png":     "png",
	"image/webp":    "webp",
	"image/gif":     "gif",
	"image/svg+xml": "svg",
}

// AllowedType reports whether a content type may be uploaded for the kind.
// SVG is only accepted for logos (menu photos have no reason to be SVG and
// SVG can carry scripts, so keep it to the trusted brand-owner path).
func AllowedType(kind Kind, contentType string) (ext string, ok bool) {
	ct := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	ext, ok = extByType[ct]
	if !ok {
		return "", false
	}
	if ct == "image/svg+xml" && kind != KindBrandLogo {
		return "", false
	}
	return ext, true
}

type Client struct {
	cfg Config
	s3  *s3.Client
}

// New returns a client, or a disabled client (Enabled()==false) when cfg is
// incomplete. It never fails at startup so a missing R2 config is a runtime
// 503 on upload rather than a crash.
func New(cfg Config) *Client {
	cfg.PublicBaseURL = strings.TrimRight(cfg.PublicBaseURL, "/")
	c := &Client{cfg: cfg}
	if !cfg.Enabled() {
		return c
	}
	c.s3 = s3.New(s3.Options{
		Region:       "auto",
		BaseEndpoint: aws.String(fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccountID)),
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		// R2 wants path-style addressing on the account endpoint.
		UsePathStyle: true,
	})
	return c
}

func (c *Client) Enabled() bool { return c.s3 != nil }

// PublicURL returns the CDN URL for an object key.
func (c *Client) PublicURL(key string) string {
	return c.cfg.PublicBaseURL + "/" + key
}

type Object struct {
	Key string `json:"key"`
	URL string `json:"url"`
}

// Put uploads body under a fresh key for the brand/kind and returns its public URL.
func (c *Client) Put(ctx context.Context, brandID uuid.UUID, kind Kind, contentType string, body io.Reader, size int64) (Object, error) {
	if !c.Enabled() {
		return Object{}, ErrNotConfigured
	}
	ext, ok := AllowedType(kind, contentType)
	if !ok {
		return Object{}, fmt.Errorf("storage: unsupported content type %q", contentType)
	}
	key := fmt.Sprintf("brands/%s/%s/%s.%s", brandID, kind, uuid.New(), ext)

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	_, err := c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.cfg.Bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
		CacheControl:  aws.String("public, max-age=31536000, immutable"),
	})
	if err != nil {
		return Object{}, fmt.Errorf("storage: put %s: %w", key, err)
	}
	return Object{Key: key, URL: c.PublicURL(key)}, nil
}

// Delete removes an object. Missing objects are not an error.
func (c *Client) Delete(ctx context.Context, key string) error {
	if !c.Enabled() {
		return ErrNotConfigured
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("storage: delete %s: %w", key, err)
	}
	return nil
}

// KeyFromURL extracts the object key from one of our public URLs, or "" if
// the URL is not under PublicBaseURL. Used to clean up replaced images.
func (c *Client) KeyFromURL(u string) string {
	prefix := c.cfg.PublicBaseURL + "/"
	if c.cfg.PublicBaseURL == "" || !strings.HasPrefix(u, prefix) {
		return ""
	}
	return strings.TrimPrefix(u, prefix)
}
