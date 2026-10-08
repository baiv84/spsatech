// Package auth отвечает за хеширование паролей и JWT-токены.
package auth

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const MinPasswordLength = 8

func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(h), err
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// dummyHash используется, чтобы время ответа на неверный логин
// не отличалось от ответа на неверный пароль.
var dummyHash, _ = HashPassword("dummy-password-for-timing")

func CheckPasswordOrDummy(hash *string, password string) bool {
	if hash == nil {
		CheckPassword(dummyHash, password)
		return false
	}
	return CheckPassword(*hash, password)
}

type Tokens struct {
	secret []byte
	ttl    time.Duration
}

func NewTokens(secret []byte, ttl time.Duration) *Tokens {
	return &Tokens{secret: secret, ttl: ttl}
}

type claims struct {
	jwt.RegisteredClaims
	// Version сверяется с users.token_version: после смены пароля старые токены перестают работать.
	Version int `json:"ver"`
}

func (t *Tokens) Issue(userID int64, version int) (string, error) {
	now := time.Now()
	c := claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(t.ttl)),
		},
		Version: version,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(t.secret)
}

// Parse проверяет подпись и срок действия и возвращает id пользователя и версию токена.
func (t *Tokens) Parse(token string) (userID int64, version int, err error) {
	var c claims
	_, err = jwt.ParseWithClaims(token, &c, func(*jwt.Token) (any, error) {
		return t.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil {
		return 0, 0, err
	}
	userID, err = strconv.ParseInt(c.Subject, 10, 64)
	if err != nil {
		return 0, 0, errors.New("invalid subject")
	}
	return userID, c.Version, nil
}
