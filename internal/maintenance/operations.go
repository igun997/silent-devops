package maintenance

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strconv"
	"sync"
	"time"
)

type Operations struct{}

func (Operations) Processes(limit uint32) []string {
	if limit == 0 || limit > 1000 {
		return nil
	}
	return []string{"ps", "-eo", "pid=,ppid=,user=,stat=,etimes=,comm=", "--sort=-etimes"}
}
func (Operations) ListServices(limit uint32) []string {
	if limit == 0 || limit > 1000 {
		return nil
	}
	return []string{"systemctl", "list-units", "--type=service", "--all", "--no-legend", "--no-pager", "--plain"}
}
func (Operations) Service(action, unit string) []string {
	if ValidateUnit(unit) != nil {
		return nil
	}
	switch action {
	case "status":
		return []string{"systemctl", "show", unit, "--no-pager", "--property=Id,LoadState,ActiveState,SubState,Description"}
	case "start", "stop", "restart":
		return []string{"systemctl", action, "--", unit}
	default:
		return nil
	}
}
func (Operations) Journal(unit string, since, until time.Time, limit uint32) []string {
	if ValidateUnit(unit) != nil || limit == 0 || limit > 10000 || since.IsZero() || until.IsZero() || !until.After(since) {
		return nil
	}
	return []string{"journalctl", "--unit", unit, "--since", "@" + strconv.FormatInt(since.Unix(), 10), "--until", "@" + strconv.FormatInt(until.Unix(), 10), "--lines", strconv.FormatUint(uint64(limit), 10), "--no-pager", "--output=short-iso"}
}

type rebootConfirmation struct {
	agentID string
	expires time.Time
}
type RebootManager struct {
	mu     sync.Mutex
	ttl    time.Duration
	tokens map[string]rebootConfirmation
}

func NewRebootManager(ttl time.Duration) *RebootManager {
	return &RebootManager{ttl: ttl, tokens: make(map[string]rebootConfirmation)}
}
func (m *RebootManager) Confirm(agentID string, now time.Time) (string, error) {
	if agentID == "" || m.ttl <= 0 {
		return "", errors.New("agent and positive TTL required")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	m.mu.Lock()
	m.tokens[token] = rebootConfirmation{agentID: agentID, expires: now.Add(m.ttl)}
	m.mu.Unlock()
	return token, nil
}
func (m *RebootManager) Consume(agentID, token string, now time.Time) error {
	m.mu.Lock()
	confirmation, ok := m.tokens[token]
	if ok && confirmation.agentID == agentID {
		delete(m.tokens, token)
	}
	m.mu.Unlock()
	if !ok || confirmation.agentID != agentID || now.After(confirmation.expires) {
		return errors.New("invalid reboot confirmation")
	}
	return nil
}
