package auth

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const issuer = "movank-local"
const audience = "movank-api"

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type Principal struct {
	UserID     string
	MerchantID string
	ExpiresAt  time.Time
}

type claims struct {
	MerchantID string `json:"merchant_id"`
	jwt.RegisteredClaims
}

func (c claims) Validate() error {
	if !uuidPattern.MatchString(c.Subject) || !uuidPattern.MatchString(c.MerchantID) {
		return errors.New("invalid identity")
	}
	if c.IssuedAt == nil || c.NotBefore == nil {
		return errors.New("missing token timestamps")
	}
	return nil
}

type Authenticator struct {
	key    []byte
	origin string
}

func New(encodedKey string, publicOrigin ...string) (*Authenticator, error) {
	key, err := hex.DecodeString(encodedKey)
	if err != nil || len(key) < 32 {
		return nil, errors.New("AUTH_SIGNING_KEY must contain at least 32 random bytes encoded as hexadecimal")
	}
	origin := ""
	if len(publicOrigin) > 0 {
		origin = strings.TrimSuffix(publicOrigin[0], "/")
	}
	if origin != "" {
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return nil, errors.New("PUBLIC_ORIGIN must be an http or https origin")
		}
	}
	return &Authenticator{key: key, origin: origin}, nil
}

func (a *Authenticator) Issue(merchantID, userID string) (string, error) {
	now := time.Now().UTC()
	c := claims{
		MerchantID: strings.ToLower(merchantID),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: issuer, Subject: strings.ToLower(userID), Audience: jwt.ClaimStrings{audience},
			IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
	}
	if err := c.Validate(); err != nil {
		return "", err
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(a.key)
}

func (a *Authenticator) Verify(raw string) (Principal, error) {
	if len(raw) == 0 || len(raw) > 4096 {
		return Principal{}, errors.New("invalid token")
	}
	c := new(claims)
	token, err := jwt.ParseWithClaims(raw, c, func(*jwt.Token) (any, error) {
		return a.key, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(issuer),
		jwt.WithAudience(audience), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil || !token.Valid {
		return Principal{}, errors.New("invalid token")
	}
	return Principal{UserID: strings.ToLower(c.Subject), MerchantID: strings.ToLower(c.MerchantID), ExpiresAt: c.ExpiresAt.Time}, nil
}

type principalKey struct{}

func FromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalKey{}).(Principal)
	return principal, ok
}

func (a *Authenticator) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		values := r.Header.Values("Authorization")
		var principal Principal
		valid := false
		if len(values) == 1 {
			parts := strings.Fields(values[0])
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				var err error
				principal, err = a.Verify(parts[1])
				valid = err == nil
			}
		}
		if len(values) == 0 {
			if cookie, err := r.Cookie(SessionCookie); err == nil {
				principal, err = a.Verify(cookie.Value)
				valid = err == nil
				if valid && r.Method != "GET" && r.Method != "HEAD" && !a.SameOrigin(r) {
					authError(w, 403, "invalid_origin")
					return
				}
			}
		}
		if !valid {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("{\"error\":\"unauthorized\"}\n"))
			return
		}
		if expected := r.Header.Get("X-Workspace"); expected != "" && expected != principal.MerchantID+":"+principal.UserID {
			authError(w, 409, "session_changed")
			return
		}
		ctx := context.WithValue(r.Context(), principalKey{}, principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
