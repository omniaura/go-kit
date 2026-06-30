package hit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// KeyPart adds one explicit cache-key dimension.
type KeyPart interface {
	appendCacheKey(ctx context.Context, req *http.Request, body []byte, b *strings.Builder) error
}

type keyPartFunc func(context.Context, *http.Request, []byte, *strings.Builder) error

func (fn keyPartFunc) appendCacheKey(
	ctx context.Context,
	req *http.Request,
	body []byte,
	b *strings.Builder,
) error {
	return fn(ctx, req, body, b)
}

// Header includes a hashed request header value in the cache key.
func Header(name string) KeyPart {
	return Headers(name)
}

// Headers includes hashed request header values in the cache key.
func Headers(names ...string) KeyPart {
	return keyPartFunc(func(ctx context.Context, req *http.Request, body []byte, b *strings.Builder) error {
		for _, name := range names {
			appendHeaderKeyPart(b, req.Header, name)
		}
		return nil
	})
}

// BodyHash includes a hash of the request body in the cache key.
func BodyHash() KeyPart {
	return keyPartFunc(func(ctx context.Context, req *http.Request, body []byte, b *strings.Builder) error {
		appendHashedKeyPart(b, "body", "", body)
		return nil
	})
}

// Field includes a stable caller-provided value in the cache key.
func Field(name, value string) KeyPart {
	return keyPartFunc(func(ctx context.Context, req *http.Request, body []byte, b *strings.Builder) error {
		appendFieldKeyPart(b, name, value)
		return nil
	})
}

// KeyFunc includes a value computed from the request in the cache key.
func KeyFunc(
	name string,
	fn func(context.Context, *http.Request, []byte) (string, error),
) KeyPart {
	return keyPartFunc(func(ctx context.Context, req *http.Request, body []byte, b *strings.Builder) error {
		value, err := fn(ctx, req, body)
		if err != nil {
			return err
		}
		appendFieldKeyPart(b, name, value)
		return nil
	})
}

func baseCacheKey(method string, u *url.URL) string {
	var b strings.Builder
	b.WriteString("hit:")
	b.WriteString(strings.ToUpper(method))
	b.WriteByte(':')
	if u.Scheme != "" || u.Host != "" {
		b.WriteString(u.Scheme)
		b.WriteString("://")
		b.WriteString(strings.ToLower(u.Host))
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	b.WriteString(path)
	if u.RawQuery != "" {
		b.WriteByte('?')
		b.WriteString(u.RawQuery)
	}
	return b.String()
}

func appendHeaderKeyPart(b *strings.Builder, headers http.Header, name string) {
	values := headerValues(headers, name)
	if len(values) == 0 {
		return
	}
	appendHashedKeyPart(b, "header", strings.ToLower(http.CanonicalHeaderKey(name)), []byte(strings.Join(values, "\x00")))
}

func appendFieldKeyPart(b *strings.Builder, name, value string) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "field"
	}
	b.WriteString("|field:")
	b.WriteString(url.QueryEscape(name))
	b.WriteByte('=')
	b.WriteString(url.QueryEscape(value))
}

func appendHashedKeyPart(b *strings.Builder, kind, name string, value []byte) {
	b.WriteByte('|')
	b.WriteString(kind)
	if name != "" {
		b.WriteByte(':')
		b.WriteString(url.QueryEscape(name))
	}
	b.WriteString("=sha256:")
	b.WriteString(sha256Hex(value))
}

func headerValues(headers http.Header, name string) []string {
	if headers == nil {
		return nil
	}
	values := append([]string(nil), headers.Values(name)...)
	sort.Strings(values)
	return values
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func keyPartError(part KeyPart) error {
	return fmt.Errorf("nil cache key part: %T", part)
}
