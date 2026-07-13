package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
	devopsv1 "silent-devops/api/devops/v1"
)

type CIDRPolicy struct{ prefixes []netip.Prefix }

func ParseCIDRs(values []string) (CIDRPolicy, error) {
	if len(values) == 0 {
		return CIDRPolicy{}, errors.New("at least one CIDR required")
	}
	p := CIDRPolicy{}
	for _, value := range values {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return CIDRPolicy{}, fmt.Errorf("invalid CIDR %q: %w", value, err)
		}
		p.prefixes = append(p.prefixes, prefix.Masked())
	}
	return p, nil
}
func (p CIDRPolicy) Allows(addr netip.Addr) bool {
	addr = addr.Unmap()
	for _, prefix := range p.prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func HashPassword(password string) (string, error) {
	if len(password) < 16 {
		return "", errors.New("password must be at least 16 bytes")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return fmt.Sprintf("argon2id$v=19$m=65536,t=3,p=2$%s$%s", hex.EncodeToString(salt), hex.EncodeToString(hash)), nil
}
func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "argon2id" {
		return false
	}
	salt, err := hex.DecodeString(parts[3])
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(parts[4])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, uint32(len(want)))
	return hmac.Equal(got, want)
}

type Claims struct {
	Subject string        `json:"sub"`
	Role    devopsv1.Role `json:"role"`
	Issued  int64         `json:"iat"`
	Expires int64         `json:"exp"`
	Nonce   string        `json:"nonce"`
}
type Issuer struct {
	key []byte
	ttl time.Duration
}

func NewIssuer(key []byte, ttl time.Duration) (*Issuer, error) {
	if len(key) < 32 {
		return nil, errors.New("token key must be at least 32 bytes")
	}
	if ttl <= 0 || ttl > time.Hour {
		return nil, errors.New("token TTL must be between zero and one hour")
	}
	return &Issuer{key: append([]byte(nil), key...), ttl: ttl}, nil
}
func (i *Issuer) Issue(subject string, role devopsv1.Role, now time.Time) (string, error) {
	if subject == "" || role == devopsv1.Role_ROLE_UNSPECIFIED {
		return "", errors.New("subject and role required")
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	claims := Claims{Subject: subject, Role: role, Issued: now.Unix(), Expires: now.Add(i.ttl).Unix(), Nonce: hex.EncodeToString(nonce)}
	body, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(body)
	return encoded + "." + i.sign(encoded), nil
}
func (i *Issuer) Verify(token string, now time.Time) (Claims, error) {
	var claims Claims
	parts := strings.Split(token, ".")
	if len(parts) != 2 || !hmac.Equal([]byte(parts[1]), []byte(i.sign(parts[0]))) {
		return claims, errors.New("invalid access token")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claims, errors.New("invalid access token")
	}
	if err := json.Unmarshal(body, &claims); err != nil {
		return claims, errors.New("invalid access token")
	}
	if claims.Expires <= now.Unix() || claims.Issued > now.Add(time.Minute).Unix() {
		return claims, errors.New("access token expired or not yet valid")
	}
	return claims, nil
}
func (i *Issuer) sign(body string) string {
	mac := hmac.New(sha256.New, i.key)
	mac.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

type bucket struct {
	start time.Time
	count int
}
type RateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]bucket
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{limit: limit, window: window, buckets: make(map[string]bucket)}
}
func (l *RateLimiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.buckets[key]
	if b.start.IsZero() || now.Sub(b.start) >= l.window {
		b = bucket{start: now}
	}
	if b.count >= l.limit {
		return false
	}
	b.count++
	l.buckets[key] = b
	return true
}

type Action int

const (
	ActionRead Action = iota
	ActionOperate
	ActionAdmin
)

func Allowed(role devopsv1.Role, action Action) bool {
	required := map[Action]devopsv1.Role{ActionRead: devopsv1.Role_ROLE_VIEWER, ActionOperate: devopsv1.Role_ROLE_OPERATOR, ActionAdmin: devopsv1.Role_ROLE_ADMIN}[action]
	return role >= required
}
func ParseBearer(header string) (string, error) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", errors.New("invalid authorization header")
	}
	return parts[1], nil
}
func ParsePort(value string) (uint16, error) {
	n, err := strconv.ParseUint(value, 10, 16)
	return uint16(n), err
}
