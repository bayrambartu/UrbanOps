package httpapi

import (
	"context"
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Claims struct {
	ID      int64  `json:"sub"`
	Role    string `json:"role"`
	Expires int64  `json:"exp"`
}
type authKey struct{}

type tokenClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

func sign(c Claims, secret string) string {
	claims := tokenClaims{Role: c.Role, RegisteredClaims: jwt.RegisteredClaims{
		Issuer: "urbanops", Subject: strconv.FormatInt(c.ID, 10),
		ExpiresAt: jwt.NewNumericDate(time.Unix(c.Expires, 0)), IssuedAt: jwt.NewNumericDate(time.Now()),
	}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		panic(err)
	} // HS256 with a byte-slice key cannot fail.
	return token
}
func verify(token, secret string) (Claims, error) {
	var parsed tokenClaims
	_, err := jwt.ParseWithClaims(token, &parsed, func(t *jwt.Token) (any, error) { return []byte(secret), nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("urbanops"), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil {
		return Claims{}, err
	}
	id, err := strconv.ParseInt(parsed.Subject, 10, 64)
	if err != nil || id <= 0 {
		return Claims{}, errors.New("invalid subject")
	}
	return Claims{id, parsed.Role, parsed.ExpiresAt.Unix()}, nil
}
func (s *Server) protected(next http.HandlerFunc, roles ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := verify(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), s.Secret)
		if err != nil {
			problem(w, 401, "Oturum gerekli")
			return
		}
		// Reload the role so role changes take effect without waiting for JWT expiry.
		if err = s.DB.QueryRow(r.Context(), `SELECT role FROM users WHERE id=$1`, c.ID).Scan(&c.Role); err != nil {
			problem(w, 401, "Kullanıcı bulunamadı")
			return
		}
		allowed := len(roles) == 0
		for _, role := range roles {
			if c.Role == role {
				allowed = true
			}
		}
		if !allowed {
			problem(w, 403, "Bu işlem için yetkiniz yok")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), authKey{}, c)))
	}
}
