// Package apple verifies Apple Sign-In identity tokens.
//
// Apple's identity token is a JWT signed by one of a rotating set of RSA keys
// published at https://appleid.apple.com/auth/keys. We:
//  1. Fetch + cache the JWKs (15-min TTL).
//  2. Look up the key matching the token's `kid` header.
//  3. Verify the RS256 signature.
//  4. Validate iss=https://appleid.apple.com, aud=<our clientID>, exp not past.
//
// See https://developer.apple.com/documentation/sign_in_with_apple/sign_in_with_apple_rest_api/verifying_a_user
package apple

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	jwksURL           = "https://appleid.apple.com/auth/keys"
	issuerURL         = "https://appleid.apple.com"
	cacheTTL          = 15 * time.Minute
	httpClientTimeout = 5 * time.Second
)

// Claims is the subset of Apple's ID-token payload we care about.
type Claims struct {
	Sub            string `json:"sub"`
	Email          string `json:"email,omitempty"`
	EmailVerified  any    `json:"email_verified,omitempty"` // string OR bool, depending on Apple's mood
	IsPrivateEmail any    `json:"is_private_email,omitempty"`
	jwt.RegisteredClaims
}

type Verifier struct {
	clientIDs []string // accepted `aud` values (Services ID + App Bundle ID, etc.)
	http      *http.Client

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	expiresAt time.Time
}

// NewVerifier accepts the list of allowed audiences (e.g. one Services ID for
// web/Android plus one App Bundle ID for native iOS sign-in).
func NewVerifier(clientIDs ...string) *Verifier {
	cleaned := make([]string, 0, len(clientIDs))
	for _, id := range clientIDs {
		if id != "" {
			cleaned = append(cleaned, id)
		}
	}
	return &Verifier{
		clientIDs: cleaned,
		http:      &http.Client{Timeout: httpClientTimeout},
		keys:      map[string]*rsa.PublicKey{},
	}
}

// Verify parses + validates the identity token, returning the trusted claims.
func (v *Verifier) Verify(ctx context.Context, idToken string) (*Claims, error) {
	if len(v.clientIDs) == 0 {
		return nil, errors.New("apple verifier: no client IDs configured")
	}

	claims := &Claims{}
	tok, err := jwt.ParseWithClaims(idToken, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("missing kid header")
		}
		return v.lookupKey(ctx, kid)
	})
	if err != nil {
		return nil, fmt.Errorf("parse apple token: %w", err)
	}
	if !tok.Valid {
		return nil, errors.New("apple token: invalid")
	}
	if claims.Issuer != issuerURL {
		return nil, fmt.Errorf("apple token: unexpected issuer %q", claims.Issuer)
	}
	if !v.audAllowed(claims.Audience) {
		return nil, fmt.Errorf("apple token: audience mismatch")
	}
	return claims, nil
}

func (v *Verifier) audAllowed(aud jwt.ClaimStrings) bool {
	for _, got := range aud {
		for _, want := range v.clientIDs {
			if got == want {
				return true
			}
		}
	}
	return false
}

func (v *Verifier) lookupKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	if time.Now().Before(v.expiresAt) {
		if k, ok := v.keys[kid]; ok {
			v.mu.RUnlock()
			return k, nil
		}
	}
	v.mu.RUnlock()

	if err := v.refresh(ctx); err != nil {
		return nil, err
	}

	v.mu.RLock()
	defer v.mu.RUnlock()
	k, ok := v.keys[kid]
	if !ok {
		return nil, fmt.Errorf("apple token: no key for kid=%s", kid)
	}
	return k, nil
}

type appleJWKs struct {
	Keys []struct {
		Kty string `json:"kty"`
		Kid string `json:"kid"`
		Use string `json:"use"`
		Alg string `json:"alg"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

func (v *Verifier) refresh(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	resp, err := v.http.Do(req)
	if err != nil {
		return fmt.Errorf("fetch apple JWKs: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("apple JWKs HTTP %d", resp.StatusCode)
	}

	var raw appleJWKs
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return fmt.Errorf("decode apple JWKs: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(raw.Keys))
	for _, k := range raw.Keys {
		if k.Kty != "RSA" {
			continue
		}
		pub, err := parseRSAPublicKey(k.N, k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = pub
	}
	if len(keys) == 0 {
		return errors.New("apple JWKs: no usable RSA keys")
	}

	v.mu.Lock()
	v.keys = keys
	v.expiresAt = time.Now().Add(cacheTTL)
	v.mu.Unlock()
	return nil
}

func parseRSAPublicKey(nB64URL, eB64URL string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64URL)
	if err != nil {
		return nil, fmt.Errorf("decode n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64URL)
	if err != nil {
		return nil, fmt.Errorf("decode e: %w", err)
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 | int(b)
	}
	if e == 0 {
		return nil, errors.New("exponent is zero")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
}
