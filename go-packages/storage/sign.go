package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strings"
	"time"
)

// request is one signed-URL request in the neutral form both adapters turn into a signature.
type request struct {
	method string
	// key is the object key; the adapter decides where it sits in the URL.
	key string
	// headers are the extra headers to sign, besides host.
	headers map[string]string
	// query are extra query parameters to sign (response-content-disposition).
	query map[string]string
	// size, when > 0, binds the upload to exactly that many bytes.
	size    int64
	expires time.Duration
}

const unsignedPayload = "UNSIGNED-PAYLOAD"

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// uriEncode is the RFC 3986 encoding both signing schemes require: unreserved characters stay,
// everything else is %XX, and "/" stays only when encodeSlash is false.
func uriEncode(s string, encodeSlash bool) string {
	const hexDigits = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		case c == '/' && !encodeSlash:
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hexDigits[c>>4])
			b.WriteByte(hexDigits[c&15])
		}
	}
	return b.String()
}

// canonicalQuery sorts and encodes query parameters by name, as both schemes define.
func canonicalQuery(q map[string]string) string {
	names := make([]string, 0, len(q))
	for k := range q {
		names = append(names, k)
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for i, k := range names {
		parts[i] = uriEncode(k, true) + "=" + uriEncode(q[k], true)
	}
	return strings.Join(parts, "&")
}

// canonicalHeaders renders the signed headers (host included) sorted by name, one "name:value\n"
// each, together with the semicolon-joined list of names.
func canonicalHeaders(host string, extra map[string]string) (headers, signed string) {
	all := map[string]string{"host": host}
	for k, v := range extra {
		all[strings.ToLower(k)] = strings.TrimSpace(v)
	}
	names := make([]string, 0, len(all))
	for k := range all {
		names = append(names, k)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, k := range names {
		b.WriteString(k + ":" + all[k] + "\n")
	}
	return b.String(), strings.Join(names, ";")
}

// buildURL joins the endpoint pieces and the signed query into the final URL.
func buildURL(scheme, host, path, query string) string {
	u := url.URL{Scheme: scheme, Host: host}
	return u.String() + uriEncode(path, false) + "?" + query
}

// contentDisposition builds the attachment header for a download name: an ASCII fallback with
// quotes, backslashes, separators and control characters replaced, plus the RFC 5987 form that
// carries the real name.
func contentDisposition(name string) string {
	var ascii strings.Builder
	var clean strings.Builder
	for _, r := range name {
		switch {
		case r < 0x20 || r == 0x7f:
			continue // never in a header, in either form
		case r == '"' || r == '\\' || r == '/' || r == ';' || r > 0x7e:
			ascii.WriteByte('_')
		default:
			ascii.WriteRune(r)
		}
		clean.WriteRune(r)
	}
	return `attachment; filename="` + ascii.String() + `"; filename*=UTF-8''` + uriEncode(clean.String(), true)
}
