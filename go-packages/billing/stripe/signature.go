// Package stripe is a skeleton billing.Provider for Stripe using only
// net/http and encoding/json. Webhook verification and parsing are complete
// for the events listed in ParseWebhook; Checkout and portal creation are a
// minimal REST call and the part most likely to need extending.
package stripe

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bernardoforcillo/ignition/go-packages/billing"
)

// DefaultTolerance is the accepted clock skew / replay window, matching
// Stripe's own libraries.
const DefaultTolerance = 5 * time.Minute

// verifySignature checks a Stripe-Signature header ("t=...,v1=...[,v1=...]")
// against HMAC-SHA256(secret, "t.payload"). Several v1 values occur during
// secret rotation; any match suffices. The timestamp must be within tolerance
// of now, which bounds replay of captured requests.
func verifySignature(payload []byte, header, secret string, now time.Time, tolerance time.Duration) error {
	var ts string
	var sigs [][]byte
	for part := range strings.SplitSeq(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			ts = v
		case "v1":
			if sig, err := hex.DecodeString(v); err == nil {
				sigs = append(sigs, sig)
			}
		}
	}
	if ts == "" || len(sigs) == 0 {
		return fmt.Errorf("%w: missing timestamp or v1 signature", billing.ErrInvalidSignature)
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return fmt.Errorf("%w: bad timestamp", billing.ErrInvalidSignature)
	}
	if d := now.Sub(time.Unix(sec, 0)); d > tolerance || d < -tolerance {
		return fmt.Errorf("%w: timestamp outside tolerance", billing.ErrInvalidSignature)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write(payload)
	want := mac.Sum(nil)
	for _, sig := range sigs {
		if hmac.Equal(sig, want) { // constant time
			return nil
		}
	}
	return fmt.Errorf("%w: no matching v1 signature", billing.ErrInvalidSignature)
}
