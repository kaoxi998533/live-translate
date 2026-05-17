package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/bcrypt"
)

var Default = New()

var (
	ErrEmailTaken         = errors.New("邮箱已注册")
	ErrInvalidCredential  = errors.New("邮箱或密码不正确")
	ErrQuotaExceeded      = errors.New("本周翻译额度已用完")
	ErrSessionNotFound    = errors.New("翻译会话不存在")
	ErrSessionNotActive   = errors.New("翻译会话已结束")
	ErrInvalidUsageSecond = errors.New("用量秒数必须大于 0")
)

type Store struct {
	mu       sync.Mutex
	db       *sql.DB
	users    map[string]User
	byEmail  map[string]string
	sessions map[string]TranslationSession
	orders   map[string]PaymentOrder
	usage    []UsageEvent
}

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"displayName"`
	Plan         string    `json:"plan"`
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
	PartyALanguage string    `json:"partyALanguage"`
	PartyBLanguage string    `json:"partyBLanguage"`
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

type PaymentOrder struct {
	ID              string    `json:"id"`
	UserID          string    `json:"userId"`
	PlanID          string    `json:"planId"`
	Provider        string    `json:"provider"`
	ProviderOrderID string    `json:"providerOrderId"`
	AmountMinor     int       `json:"amountMinor"`
	Currency        string    `json:"currency"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"createdAt"`
}

func New() *Store {
	return &Store{
		users:    make(map[string]User),
		byEmail:  make(map[string]string),
		sessions: make(map[string]TranslationSession),
		orders:   make(map[string]PaymentOrder),
	}
}

func (s *Store) Connect(ctx context.Context, databaseURL string, migrationPath string) error {
	if databaseURL == "" {
		return nil
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(12)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return err
	}
	if migrationPath != "" {
		if err := runMigration(ctx, db, "001_initial", migrationPath); err != nil {
			_ = db.Close()
			return err
		}
	}

	s.mu.Lock()
	s.db = db
	s.mu.Unlock()
	return nil
}

func (s *Store) Ping(ctx context.Context) error {
	s.mu.Lock()
	db := s.db
	s.mu.Unlock()
	if db == nil {
		return nil
	}
	return db.PingContext(ctx)
}

func (s *Store) Register(email string, password string, displayName string) (User, error) {
	passwordHash, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		return s.registerDB(email, passwordHash, displayName)
	}

	if _, exists := s.byEmail[email]; exists {
		return User{}, ErrEmailTaken
	}

	now := time.Now().UTC()
	user := User{
		ID:           randomID("usr"),
		Email:        email,
		DisplayName:  displayName,
		Plan:         "trial",
		PasswordHash: passwordHash,
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

	if s.db != nil {
		return s.authenticateDB(email, password)
	}

	id, exists := s.byEmail[email]
	if !exists {
		return User{}, ErrInvalidCredential
	}

	user := s.users[id]
	if !validPassword(user.PasswordHash, password) {
		return User{}, ErrInvalidCredential
	}

	return user, nil
}

func (s *Store) User(id string) (User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		user, err := s.userDB(id)
		return user, err == nil
	}

	user, exists := s.users[id]
	return user, exists
}

func (s *Store) Entitlement(userID string) Entitlement {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if s.db != nil {
		entitlement, err := s.entitlementDB(userID, now)
		if err == nil {
			return entitlement
		}
		return emptyEntitlement(now)
	}

	return s.entitlementLocked(userID, now)
}

func (s *Store) CreateSession(userID string, partyALanguage string, partyBLanguage string, inputMode string) (TranslationSession, Entitlement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if s.db != nil {
		entitlement, err := s.entitlementDB(userID, now)
		if err != nil {
			return TranslationSession{}, emptyEntitlement(now), err
		}
		if !entitlement.CanTranslate {
			return TranslationSession{}, entitlement, ErrQuotaExceeded
		}
		session, err := s.createSessionDB(userID, partyALanguage, partyBLanguage, inputMode)
		return session, entitlement, err
	}

	entitlement := s.entitlementLocked(userID, now)
	if !entitlement.CanTranslate {
		return TranslationSession{}, entitlement, ErrQuotaExceeded
	}

	session := TranslationSession{
		ID:             randomID("trs"),
		UserID:         userID,
		PartyALanguage: partyALanguage,
		PartyBLanguage: partyBLanguage,
		InputMode:      inputMode,
		Status:         "active",
		StartedAt:      now,
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

	now := time.Now().UTC()
	if s.db != nil {
		return s.addUsageDB(userID, sessionID, seconds, now)
	}

	session, exists := s.sessions[sessionID]
	if !exists || session.UserID != userID {
		return Entitlement{}, ErrSessionNotFound
	}
	if session.Status != "active" {
		return Entitlement{}, ErrSessionNotActive
	}

	entitlement := s.entitlementLocked(userID, now)
	if entitlement.RemainingSeconds < seconds {
		return entitlement, ErrQuotaExceeded
	}

	s.usage = append(s.usage, UsageEvent{
		ID:        randomID("use"),
		UserID:    userID,
		SessionID: sessionID,
		Seconds:   seconds,
		CreatedAt: now,
	})

	return s.entitlementLocked(userID, now), nil
}

func (s *Store) EndSession(userID string, sessionID string) (TranslationSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		return s.endSessionDB(userID, sessionID)
	}

	session, exists := s.sessions[sessionID]
	if !exists || session.UserID != userID {
		return TranslationSession{}, ErrSessionNotFound
	}

	session.Status = "ended"
	session.EndedAt = time.Now().UTC()
	s.sessions[sessionID] = session
	return session, nil
}

func (s *Store) CreatePaymentOrder(userID string, planID string, provider string) (PaymentOrder, error) {
	if planID == "" {
		planID = "premium"
	}
	if provider == "" {
		provider = "wechat_pay"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		return s.createPaymentOrderDB(userID, planID, provider)
	}

	now := time.Now().UTC()
	order := PaymentOrder{
		ID:              randomID("ord"),
		UserID:          userID,
		PlanID:          planID,
		Provider:        provider,
		ProviderOrderID: randomID(provider),
		AmountMinor:     planAmountMinor(planID),
		Currency:        planCurrency(provider),
		Status:          "pending",
		CreatedAt:       now,
	}
	s.orders[order.ID] = order
	return order, nil
}

func (s *Store) MarkPaymentOrderPaid(userID string, orderID string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		return s.markPaymentOrderPaidDB(userID, orderID)
	}

	order, exists := s.orders[orderID]
	if !exists || order.UserID != userID {
		return User{}, ErrSessionNotFound
	}
	order.Status = "paid"
	s.orders[order.ID] = order

	user := s.users[userID]
	user.Plan = order.PlanID
	s.users[userID] = user
	return user, nil
}

func (s *Store) registerDB(email string, passwordHash string, displayName string) (User, error) {
	now := time.Now().UTC()
	trialEndsAt := now.Add(24 * time.Hour)
	var user User
	err := s.db.QueryRow(`
		insert into users (auth_subject, email, password_hash, display_name, plan_id, trial_ends_at, created_at, updated_at)
		values ($1, $1, $2, $3, 'trial', $4, $5, $5)
		returning id::text, email, coalesce(display_name, ''), plan_id, password_hash, trial_ends_at, created_at
	`, email, passwordHash, displayName, trialEndsAt, now).Scan(
		&user.ID,
		&user.Email,
		&user.DisplayName,
		&user.Plan,
		&user.PasswordHash,
		&user.TrialEndsAt,
		&user.CreatedAt,
	)
	if isUniqueViolation(err) {
		return User{}, ErrEmailTaken
	}
	return user, err
}

func (s *Store) authenticateDB(email string, password string) (User, error) {
	var user User
	err := s.db.QueryRow(`
		select id::text, email, coalesce(display_name, ''), plan_id, password_hash, trial_ends_at, created_at
		from users
		where email = $1
	`, email).Scan(
		&user.ID,
		&user.Email,
		&user.DisplayName,
		&user.Plan,
		&user.PasswordHash,
		&user.TrialEndsAt,
		&user.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) || !validPassword(user.PasswordHash, password) {
		return User{}, ErrInvalidCredential
	}
	return user, err
}

func (s *Store) userDB(id string) (User, error) {
	var user User
	err := s.db.QueryRow(`
		select id::text, email, coalesce(display_name, ''), plan_id, password_hash, trial_ends_at, created_at
		from users
		where id = $1::uuid
	`, id).Scan(
		&user.ID,
		&user.Email,
		&user.DisplayName,
		&user.Plan,
		&user.PasswordHash,
		&user.TrialEndsAt,
		&user.CreatedAt,
	)
	return user, err
}

func (s *Store) createSessionDB(userID string, partyALanguage string, partyBLanguage string, inputMode string) (TranslationSession, error) {
	var session TranslationSession
	err := s.db.QueryRow(`
		insert into translation_sessions (user_id, party_a_language, party_b_language, input_mode, status)
		values ($1::uuid, $2, $3, $4, 'active')
		returning id::text, user_id::text, party_a_language, party_b_language, input_mode, status, started_at
	`, userID, partyALanguage, partyBLanguage, inputMode).Scan(
		&session.ID,
		&session.UserID,
		&session.PartyALanguage,
		&session.PartyBLanguage,
		&session.InputMode,
		&session.Status,
		&session.StartedAt,
	)
	return session, err
}

func (s *Store) addUsageDB(userID string, sessionID string, seconds int, now time.Time) (Entitlement, error) {
	var status string
	err := s.db.QueryRow(`
		select status
		from translation_sessions
		where id = $1::uuid and user_id = $2::uuid
	`, sessionID, userID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return Entitlement{}, ErrSessionNotFound
	}
	if err != nil {
		return Entitlement{}, err
	}
	if status != "active" {
		return Entitlement{}, ErrSessionNotActive
	}

	entitlement, err := s.entitlementDB(userID, now)
	if err != nil {
		return Entitlement{}, err
	}
	if entitlement.RemainingSeconds < seconds {
		return entitlement, ErrQuotaExceeded
	}

	_, err = s.db.Exec(`
		insert into usage_events (user_id, translation_session_id, event_type, seconds, metadata)
		values ($1::uuid, $2::uuid, 'translation_seconds', $3, '{}'::jsonb)
	`, userID, sessionID, seconds)
	if err != nil {
		return Entitlement{}, err
	}

	return s.entitlementDB(userID, now)
}

func (s *Store) endSessionDB(userID string, sessionID string) (TranslationSession, error) {
	var session TranslationSession
	var endedAt sql.NullTime
	err := s.db.QueryRow(`
		update translation_sessions
		set status = 'ended', ended_at = now()
		where id = $1::uuid and user_id = $2::uuid
		returning id::text, user_id::text, party_a_language, party_b_language, input_mode, status, started_at, ended_at
	`, sessionID, userID).Scan(
		&session.ID,
		&session.UserID,
		&session.PartyALanguage,
		&session.PartyBLanguage,
		&session.InputMode,
		&session.Status,
		&session.StartedAt,
		&endedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return TranslationSession{}, ErrSessionNotFound
	}
	if endedAt.Valid {
		session.EndedAt = endedAt.Time
	}
	return session, err
}

func (s *Store) createPaymentOrderDB(userID string, planID string, provider string) (PaymentOrder, error) {
	now := time.Now().UTC()
	order := PaymentOrder{
		ProviderOrderID: randomID(provider),
		AmountMinor:     planAmountMinor(planID),
		Currency:        planCurrency(provider),
		Status:          "pending",
	}
	err := s.db.QueryRow(`
		insert into payment_orders (user_id, plan_id, provider, provider_order_id, amount_minor, currency, status, metadata, created_at, updated_at)
		values ($1::uuid, $2, $3, $4, $5, $6, $7, '{}'::jsonb, $8, $8)
		returning id::text, user_id::text, plan_id, provider::text, provider_order_id, amount_minor, currency, status, created_at
	`, userID, planID, provider, order.ProviderOrderID, order.AmountMinor, order.Currency, order.Status, now).Scan(
		&order.ID,
		&order.UserID,
		&order.PlanID,
		&order.Provider,
		&order.ProviderOrderID,
		&order.AmountMinor,
		&order.Currency,
		&order.Status,
		&order.CreatedAt,
	)
	return order, err
}

func (s *Store) markPaymentOrderPaidDB(userID string, orderID string) (User, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()

	var order PaymentOrder
	err = tx.QueryRow(`
		update payment_orders
		set status = 'paid', updated_at = now()
		where id = $1::uuid and user_id = $2::uuid
		returning id::text, user_id::text, plan_id, provider::text, provider_order_id, amount_minor, currency, status, created_at
	`, orderID, userID).Scan(
		&order.ID,
		&order.UserID,
		&order.PlanID,
		&order.Provider,
		&order.ProviderOrderID,
		&order.AmountMinor,
		&order.Currency,
		&order.Status,
		&order.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrSessionNotFound
	}
	if err != nil {
		return User{}, err
	}

	_, err = tx.Exec(`
		insert into subscriptions (
			user_id, plan_id, provider, provider_customer_id, provider_subscription_id,
			status, current_period_start, current_period_end, created_at, updated_at
		)
		values ($1::uuid, $2, $3, $1, $4, 'active', now(), now() + interval '30 days', now(), now())
		on conflict (provider, provider_subscription_id) do update
		set status = 'active', current_period_end = excluded.current_period_end, updated_at = now()
	`, userID, order.PlanID, order.Provider, order.ProviderOrderID)
	if err != nil {
		return User{}, err
	}

	_, err = tx.Exec(`update users set plan_id = $2, updated_at = now() where id = $1::uuid`, userID, order.PlanID)
	if err != nil {
		return User{}, err
	}

	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return s.userDB(userID)
}

func (s *Store) entitlementDB(userID string, now time.Time) (Entitlement, error) {
	user, err := s.userDB(userID)
	if err != nil {
		return Entitlement{}, err
	}

	start, end := weekBounds(now)
	var used int
	err = s.db.QueryRow(`
		select coalesce(sum(seconds), 0)::int
		from usage_events
		where user_id = $1::uuid and created_at >= $2 and created_at < $3
	`, userID, start, end).Scan(&used)
	if err != nil {
		return Entitlement{}, err
	}

	premiumActive, err := s.premiumActiveDB(userID, now)
	if err != nil {
		return Entitlement{}, err
	}

	plan := user.Plan
	limit := 0
	trialActive := now.Before(user.TrialEndsAt)
	switch {
	case premiumActive || user.Plan == "premium":
		plan = "premium"
		limit = s.planLimitDB("premium", 18000)
	case trialActive:
		plan = "trial"
		limit = s.planLimitDB("trial", 900)
	default:
		plan = "trial"
		limit = 0
	}

	return buildEntitlement(plan, trialActive, limit, used, start, end), nil
}

func (s *Store) premiumActiveDB(userID string, now time.Time) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`
		select exists (
			select 1
			from subscriptions
			where user_id = $1::uuid
			  and plan_id = 'premium'
			  and status in ('active', 'trialing')
			  and (current_period_end is null or current_period_end > $2)
		)
	`, userID, now).Scan(&exists)
	return exists, err
}

func (s *Store) planLimitDB(planID string, fallback int) int {
	var limit int
	if err := s.db.QueryRow(`select weekly_limit_seconds from plans where id = $1`, planID).Scan(&limit); err != nil {
		return fallback
	}
	return limit
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

	return buildEntitlement(user.Plan, trialActive, limit, used, start, end)
}

func buildEntitlement(plan string, trialActive bool, limit int, used int, start time.Time, end time.Time) Entitlement {
	remaining := limit - used
	if remaining < 0 {
		remaining = 0
	}

	result := Entitlement{
		CanTranslate:       remaining > 0,
		Plan:               plan,
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

func emptyEntitlement(now time.Time) Entitlement {
	start, end := weekBounds(now)
	return buildEntitlement("trial", false, 0, 0, start, end)
}

func weekBounds(now time.Time) (time.Time, time.Time) {
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(weekday - 1))
	return start, start.AddDate(0, 0, 7)
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func validPassword(hash string, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func randomID(prefix string) string {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(buf[:])
}

func runMigration(ctx context.Context, db *sql.DB, version string, path string) error {
	if _, err := db.ExecContext(ctx, `
		create table if not exists schema_migrations (
			version text primary key,
			applied_at timestamptz not null default now()
		)
	`); err != nil {
		return err
	}

	var applied bool
	if err := db.QueryRowContext(ctx, `select exists(select 1 from schema_migrations where version = $1)`, version).Scan(&applied); err != nil {
		return err
	}
	if applied {
		return nil
	}

	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, string(body)); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, `insert into schema_migrations (version) values ($1)`, version); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SQLSTATE 23505")
}

func planAmountMinor(planID string) int {
	if planID == "premium" {
		return 3900
	}
	return 0
}

func planCurrency(provider string) string {
	if provider == "wechat_pay" || provider == "alipay" {
		return "CNY"
	}
	return "USD"
}
