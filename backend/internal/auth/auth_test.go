package auth

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const merchantA = "11111111-1111-4111-8111-111111111111"
const userA = "22222222-2222-4222-8222-222222222222"

func TestSigningKey(t *testing.T) {
	for _, key := range []string{"", "private", strings.Repeat("ab", 31)} {
		if _, err := New(key); err == nil {
			t.Fatal("accepted invalid signing key")
		}
	}
}

func TestIssueAndVerify(t *testing.T) {
	a, err := New(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := a.Issue(merchantA, userA)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := a.Verify(raw)
	if err != nil || principal.MerchantID != merchantA || principal.UserID != userA {
		t.Fatalf("unexpected principal: %v, %v", principal, err)
	}
	if _, err := a.Issue("not-a-uuid", userA); err == nil {
		t.Fatal("issued token with invalid merchant")
	}
}

func TestRejectInvalidTokens(t *testing.T) {
	a, _ := New(strings.Repeat("ab", 32))
	for _, tc := range []struct {
		name   string
		change func(*claims)
	}{
		{"expired", func(c *claims) { c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute)) }},
		{"missing expiration", func(c *claims) { c.ExpiresAt = nil }},
		{"wrong issuer", func(c *claims) { c.Issuer = "another-issuer" }},
		{"missing issuer", func(c *claims) { c.Issuer = "" }},
		{"wrong audience", func(c *claims) { c.Audience = jwt.ClaimStrings{"another-api"} }},
		{"missing audience", func(c *claims) { c.Audience = nil }},
		{"missing merchant", func(c *claims) { c.MerchantID = "" }},
		{"invalid merchant", func(c *claims) { c.MerchantID = "invalid" }},
		{"missing user", func(c *claims) { c.Subject = "" }},
		{"missing issued at", func(c *claims) { c.IssuedAt = nil }},
		{"future issued at", func(c *claims) { c.IssuedAt = jwt.NewNumericDate(time.Now().Add(time.Hour)) }},
		{"not active", func(c *claims) { c.NotBefore = jwt.NewNumericDate(time.Now().Add(time.Hour)) }},
		{"missing not before", func(c *claims) { c.NotBefore = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			c := claims{MerchantID: merchantA, RegisteredClaims: jwt.RegisteredClaims{
				Issuer: issuer, Subject: userA, Audience: jwt.ClaimStrings{audience},
				ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)), IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now),
			}}
			tc.change(&c)
			raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(a.key)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := a.Verify(raw); err == nil {
				t.Fatal("accepted invalid claims")
			}
		})
	}
	valid, _ := a.Issue(merchantA, userA)
	parts := strings.Split(valid, ".")
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	parts[1] = base64.RawURLEncoding.EncodeToString([]byte(strings.ReplaceAll(string(payload), merchantA, userA)))
	wrongKey, _ := New(strings.Repeat("cd", 32))
	wrongSignature, _ := wrongKey.Issue(merchantA, userA)
	for _, raw := range []string{"invalid", strings.Repeat("a", 4097), strings.Join(parts, "."), wrongSignature} {
		if _, err := a.Verify(raw); err == nil {
			t.Fatal("accepted invalid signature or encoding")
		}
	}
	for _, method := range []jwt.SigningMethod{jwt.SigningMethodHS384, jwt.SigningMethodNone} {
		var key any = a.key
		if method == jwt.SigningMethodNone {
			key = jwt.UnsafeAllowNoneSignatureType
		}
		now := time.Now().Unix()
		raw, err := jwt.NewWithClaims(method, jwt.MapClaims{
			"merchant_id": merchantA, "sub": userA, "iss": issuer, "aud": audience,
			"iat": now, "nbf": now, "exp": now + 900,
		}).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.Verify(raw); err == nil {
			t.Fatal("accepted unexpected algorithm")
		}
	}
}

func TestRequireRejectsInvalidHeaders(t *testing.T) {
	a, _ := New(strings.Repeat("ab", 32))
	valid, _ := a.Issue(merchantA, userA)
	for _, headers := range [][]string{nil, {""}, {"Bearer"}, {"Basic " + valid}, {"Bearer invalid"}, {"Bearer " + valid, "Bearer " + valid}} {
		r := httptest.NewRequest("GET", "/", nil)
		for _, header := range headers {
			r.Header.Add("Authorization", header)
		}
		w := httptest.NewRecorder()
		a.Require(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("unauthorized request reached handler") })).ServeHTTP(w, r)
		if w.Code != 401 || w.Header().Get("WWW-Authenticate") != "Bearer" {
			t.Fatalf("unexpected auth response: %d", w.Code)
		}
	}
}
