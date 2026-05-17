package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

var Default = New()

var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredential  = errors.New("invalid email or password")
	ErrQuotaExceeded      = errors.New("weekly quota exceeded")
	ErrSessionNotFound    = errors.New("translation session not found")
	ErrSessionNotActive   = errors.New("translation session is not active")
	ErrInvalidUsageSecond = errors.New("usage seconds must be positive")
)

type Store struct {
	mu       sync.Mutex
	users    map[string]User
	byEmail  map[string]string
	sessions map[string]TranslationSession
	usage    []UsageEvent
}

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"displayName"`
	Plan         string    `json:"plan"`
	PasswordSalt string    `json:"-"`
	PasswordHash string    `json:"-"`
	TrialEndsAt  time.Time `json:"trialEndsAt"`
	CreatedAt    time.Time `json:"createdAt"`
}

type Entitlement struct {
	CanTranslate       bool      `json:"canTranslate"`
	Reason             string    `json:"reason,omitempty"`
	Plan               string    `json:"plan"`
	TrialActive        bool      `json:"trialActive"`
	WeeklyLimitSeconds int       `json:"weeklyLimitSeconds"`
	UsedSeconds        int       `json:"usedSeconds"`
	RemainingSeconds   int       `json:"remainingSeconds"`
	PeriodStart        time.Time `json:"periodStart"`
	PeriodEnd          time.Time `json:"periodEnd"`
}

type TranslationSession struct {
	ID             string    `json:"id"`
	UserID         string    `json:"userId"`
	SourceLanguage string    `json:"sourceLanguage"`
	TargetLanguage string    `json:"targetLanguage"`
	InputMode      string    `json:"inputMode"`
	Status         string    `json:"status"`
	StartedAt      time.Time `json:"startedAt"`
	EndedAt        time.Time `json:"endedAt,omitempty"`
}

type UsageEvent struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	SessionID string    `json:"sessionId"`
	Seconds   int       `json:"seconds"`
	CreatedAt time.Time `json:"createdAt"`
}

func New() *Store {
	return &Store{
		users:    make(map[string]User),
		byEmail:  make(map[string]string),
		sessions: make(map[string]TranslationSession),
	}
}

func (s *Store) Register(email string, password string, displayName string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.byEmail[email]; exists {
		return User{}, ErrEmailTaken
	}

	now := time.Now().UTC()
	salt := randomID("salt")
	user := User{
		ID:           randomID("usr"),
		Email:        email,
		DisplayName:  displayName,
		Plan:         "trial",
		PasswordSalt: salt,
		PasswordHash: hashPassword(salt, password),
		TrialEndsAt:  now.Add(24 * time.Hour),
		CreatedAt:    now,
	}

	s.users[user.ID] = user
	s.byEmail[email] = user.ID
	return user, nil
}

func (s *Store) Authenticate(email string, password string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, exists := s.byEmail[email]
	if !exists {
		return User{}, ErrInvalidCredential
	}

	user := s.users[id]
	if user.PasswordHash != hashPassword(user.PasswordSalt, password) {
		return User{}, ErrInvalidCredential
	}

	return user, nil
}

func (s *Store) User(id string) (User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, exists := s.users[id]
	return user, exists
}

func (s *Store) Entitlement(userID string) Entitlement {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.entitlementLocked(userID, time.Now().UTC())
}

func (s *Store) CreateSession(userID string, sourceLanguage string, targetLanguage string, inputMode string) (TranslationSession, Entitlement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entitlement := s.entitlementLocked(userID, time.Now().UTC())
	if !entitlement.CanTranslate {
		return TranslationSession{}, entitlement, ErrQuotaExceeded
	}

	session := TranslationSession{
		ID:             randomID("trs"),
		UserID:         userID,
		SourceLanguage: sourceLanguage,
		TargetLanguage: targetLanguage,
		InputMode:      inputMode,
		Status:         "active",
		StartedAt:      time.Now().UTC(),
	}
	s.sessions[session.ID] = session
	return session, entitlement, nil
}

func (s *Store) AddUsage(userID string, sessionID string, seconds int) (Entitlement, error) {
	if seconds <= 0 {
		return Entitlement{}, ErrInvalidUsageSecond
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists || session.UserID != userID {
		return Entitlement{}, ErrSessionNotFound
	}
	if session.Status != "active" {
		return Entitlement{}, ErrSessionNotActive
	}

	entitlement := s.entitlementLocked(userID, time.Now().UTC())
	if entitlement.RemainingSeconds < seconds {
		return entitlement, ErrQuotaExceeded
	}

	s.usage = append(s.usage, UsageEvent{
		ID:        randomID("use"),
		UserID:    userID,
		SessionID: sessionID,
		Seconds:   seconds,
		CreatedAt: time.Now().UTC(),
	})

	return s.entitlementLocked(userID, time.Now().UTC()), nil
}

func (s *Store) EndSession(userID string, sessionID string) (TranslationSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists || session.UserID != userID {
		return TranslationSession{}, ErrSessionNotFound
	}

	session.Status = "ended"
	session.EndedAt = time.Now().UTC()
	s.sessions[sessionID] = session
	return session, nil
}

func (s *Store) entitlementLocked(userID string, now time.Time) Entitlement {
	user := s.users[userID]
	start, end := weekBounds(now)
	used := 0
	for _, event := range s.usage {
		if event.UserID == userID && !event.CreatedAt.Before(start) && event.CreatedAt.Before(end) {
			used += event.Seconds
		}
	}

	limit := 0
	trialActive := now.Before(user.TrialEndsAt)
	switch {
	case user.Plan == "premium":
		limit = 18000
	case trialActive:
		limit = 900
	default:
		limit = 0
	}

	remaining := limit - used
	if remaining < 0 {
		remaining = 0
	}

	result := Entitlement{
		CanTranslate:       remaining > 0,
		Plan:               user.Plan,
		TrialActive:        trialActive,
		WeeklyLimitSeconds: limit,
		UsedSeconds:        used,
		RemainingSeconds:   remaining,
		PeriodStart:        start,
		PeriodEnd:          end,
	}
	if !result.CanTranslate {
		result.Reason = "quota_exceeded"
		if limit == 0 {
			result.Reason = "subscription_required"
		}
	}
	return result
}

func weekBounds(now time.Time) (time.Time, time.Time) {
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(weekday - 1))
	return start, start.AddDate(0, 0, 7)
}

func hashPassword(salt string, password string) string {
	sum := sha256.Sum256([]byte(salt + ":" + password))
	return hex.EncodeToString(sum[:])
}

func randomID(prefix string) string {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(buf[:])
}
