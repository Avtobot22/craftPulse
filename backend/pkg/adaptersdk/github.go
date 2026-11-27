package adaptersdk

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

func classifyGitHubStatus(resp *http.Response, now time.Time) *Error {
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
		return classifyStatus(resp, now)
	}
	var body struct {
		Message string `json:"message"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body)
	retry := retryAfter(resp.Header.Get("Retry-After"), now)
	primary := resp.Header.Get("X-RateLimit-Remaining") == "0"
	secondary := strings.Contains(strings.ToLower(body.Message), "secondary rate limit")
	if resp.StatusCode == http.StatusForbidden && !primary && !secondary && retry == nil {
		return classifyStatus(resp, now)
	}
	if retry == nil && primary && !secondary {
		if epoch, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil && epoch > now.Unix() {
			at := time.Unix(epoch, 0).UTC()
			retry = &at
		}
	}
	if retry == nil {
		at := now.Add(time.Minute).UTC()
		retry = &at
	}
	return &Error{Code: AdapterErrorCodeRateLimited, Detail: "GitHub request rate limited", RetryAt: retry}
}

// SignGitHubAppJWT signs the short-lived credential used to request installation tokens.
func SignGitHubAppJWT(clientID, privateKey string, now time.Time) (string, error) {
	invalid := NewError(AdapterErrorCodeAuth, "invalid GitHub App private key")
	block, _ := pem.Decode([]byte(privateKey))
	if block == nil || block.Type != "RSA PRIVATE KEY" {
		return "", invalid
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil || key.N.BitLen() < 2048 {
		return "", invalid
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, nil)
	if err != nil {
		return "", invalid
	}
	token, err := jwt.Signed(signer).Claims(jwt.Claims{
		Issuer: clientID, IssuedAt: jwt.NewNumericDate(now.Add(-time.Minute)), Expiry: jwt.NewNumericDate(now.Add(9 * time.Minute)),
	}).Serialize()
	if err != nil {
		return "", invalid
	}
	return token, nil
}
