package auth_test

import (
	"context"
	"testing"
)

// A suppressed address gets no OTP, even though it's an allowlisted user —
// RequestCode no-ops silently (no mailer call), same as unknown/throttled.
func TestSuppressedAddressSkipsOTP(t *testing.T) {
	db := newDB(t)
	mailer := &captureMailer{}
	svc := newSvc(db, mailer)
	ctx := context.Background()
	email := newUser(t, db)

	if err := svc.Suppress(ctx, email, "bounce", "Permanent"); err != nil {
		t.Fatalf("suppress: %v", err)
	}
	suppressed, err := svc.IsSuppressed(ctx, email)
	if err != nil || !suppressed {
		t.Fatalf("IsSuppressed = %v, %v; want true, nil", suppressed, err)
	}
	if err := svc.RequestCode(ctx, email); err != nil {
		t.Fatalf("RequestCode: %v", err)
	}
	if len(mailer.codes) != 0 {
		t.Fatalf("sent %d codes to suppressed address; want 0", len(mailer.codes))
	}
}

// Suppress upserts (idempotent) and is case/space-insensitive on the address,
// matching RequestCode's normalizeEmail.
func TestSuppressNormalizesAndUpserts(t *testing.T) {
	db := newDB(t)
	svc := newSvc(db, &captureMailer{})
	ctx := context.Background()

	if err := svc.Suppress(ctx, "  USER@Example.test ", "bounce", "Permanent"); err != nil {
		t.Fatalf("suppress 1: %v", err)
	}
	if err := svc.Suppress(ctx, "user@example.test", "complaint", ""); err != nil {
		t.Fatalf("suppress 2 (upsert): %v", err)
	}
	suppressed, err := svc.IsSuppressed(ctx, "User@Example.Test")
	if err != nil || !suppressed {
		t.Fatalf("IsSuppressed = %v, %v; want true, nil", suppressed, err)
	}
}

// An unknown reason is rejected before touching the DB.
func TestSuppressRejectsBadReason(t *testing.T) {
	svc := newSvc(newDB(t), &captureMailer{})
	if err := svc.Suppress(context.Background(), "x@y.test", "spam", ""); err == nil {
		t.Fatal("want error for bad reason, got nil")
	}
}
