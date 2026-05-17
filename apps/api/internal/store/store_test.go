package store

import "testing"

func TestMemoryStoreRegisterAuthenticateAndQuota(t *testing.T) {
	s := New()

	user, err := s.Register("test@example.com", "password123", "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := s.Authenticate("test@example.com", "password123"); err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	session, entitlement, err := s.CreateSession(user.ID, "zh", "en", "microphone")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if entitlement.RemainingSeconds != 900 {
		t.Fatalf("remaining seconds = %d, want 900", entitlement.RemainingSeconds)
	}

	next, err := s.AddUsage(user.ID, session.ID, 60)
	if err != nil {
		t.Fatalf("add usage: %v", err)
	}
	if next.RemainingSeconds != 840 {
		t.Fatalf("remaining seconds = %d, want 840", next.RemainingSeconds)
	}
}

func TestMemoryStorePaymentOrderMarksPremium(t *testing.T) {
	s := New()

	user, err := s.Register("buyer@example.com", "password123", "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	order, err := s.CreatePaymentOrder(user.ID, "premium", "wechat_pay")
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if order.Status != "pending" {
		t.Fatalf("order status = %s, want pending", order.Status)
	}

	nextUser, err := s.MarkPaymentOrderPaid(user.ID, order.ID)
	if err != nil {
		t.Fatalf("mark paid: %v", err)
	}
	if nextUser.Plan != "premium" {
		t.Fatalf("plan = %s, want premium", nextUser.Plan)
	}

	entitlement := s.Entitlement(user.ID)
	if entitlement.WeeklyLimitSeconds != 18000 {
		t.Fatalf("weekly limit = %d, want 18000", entitlement.WeeklyLimitSeconds)
	}
}
