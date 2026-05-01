package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"kcnotes/internal/domain"
)

func TestFirstAdminWithPasskeyOnlyWhenEmpty(t *testing.T) {
	store := newTestAuthStore(t)
	ctx := context.Background()
	user := domain.User{ID: "u-1", Email: "admin@example.com", Role: domain.RoleAdmin}
	credential := testPasskey("c-1", user.ID, []byte("credential-1"))

	if err := store.CreateFirstAdminWithPasskey(ctx, user, credential); err != nil {
		t.Fatalf("create first admin: %v", err)
	}
	if err := store.CreateFirstAdminWithPasskey(ctx, domain.User{ID: "u-2", Email: "other@example.com", Role: domain.RoleAdmin}, testPasskey("c-2", "u-2", []byte("credential-2"))); err == nil {
		t.Fatal("expected second first-admin setup to fail")
	}
}

func TestPasskeyDeleteKeepsLastCredential(t *testing.T) {
	store := newTestAuthStore(t)
	ctx := context.Background()
	user := domain.User{ID: "u-1", Email: "admin@example.com", Role: domain.RoleAdmin}
	mustNoErr(t, store.CreateUser(ctx, user))
	mustNoErr(t, store.CreatePasskeyCredential(ctx, testPasskey("c-1", user.ID, []byte("credential-1"))))

	if _, err := store.DeletePasskeyCredential(ctx, user.ID, "c-1"); err != ErrConflict {
		t.Fatalf("expected ErrConflict deleting last passkey, got %v", err)
	}
	mustNoErr(t, store.CreatePasskeyCredential(ctx, testPasskey("c-2", user.ID, []byte("credential-2"))))
	deleted, err := store.DeletePasskeyCredential(ctx, user.ID, "c-1")
	if err != nil {
		t.Fatalf("delete extra passkey: %v", err)
	}
	if !deleted {
		t.Fatal("expected passkey to be deleted")
	}
}

func TestEnrollmentInvitationExpiresAndCompletesOnce(t *testing.T) {
	store := newTestAuthStore(t)
	ctx := context.Background()
	admin := domain.User{ID: "admin", Email: "admin@example.com", Role: domain.RoleAdmin}
	mustNoErr(t, store.CreateUser(ctx, admin))
	invited := domain.User{ID: "u-2", Email: "new@example.com", Role: domain.RoleEditor, Disabled: true}
	invite := domain.EnrollmentInvitation{ID: "i-1", UserID: invited.ID, TokenHash: "hash", ExpiresAt: time.Now().UTC().Add(time.Hour), CreatedBy: admin.ID}
	mustNoErr(t, store.CreateEnrollmentInvitation(ctx, invited, invite))

	gotInvite, gotUser, err := store.GetEnrollmentInvitationByTokenHash(ctx, "hash", time.Now().UTC())
	if err != nil {
		t.Fatalf("get invite: %v", err)
	}
	if gotInvite.ID != invite.ID || gotUser.ID != invited.ID {
		t.Fatalf("unexpected invite/user: %#v %#v", gotInvite, gotUser)
	}
	mustNoErr(t, store.CompleteEnrollmentInvitation(ctx, invite.ID, testPasskey("c-1", invited.ID, []byte("credential-1"))))
	if _, _, err := store.GetEnrollmentInvitationByTokenHash(ctx, "hash", time.Now().UTC()); err != ErrNotFound {
		t.Fatalf("expected used invite to be unavailable, got %v", err)
	}

	expiredUser := domain.User{ID: "u-expired", Email: "expired@example.com", Role: domain.RoleEditor, Disabled: true}
	expired := domain.EnrollmentInvitation{ID: "i-expired", UserID: expiredUser.ID, TokenHash: "expired-hash", ExpiresAt: time.Now().UTC().Add(-time.Minute), CreatedBy: admin.ID}
	mustNoErr(t, store.CreateEnrollmentInvitation(ctx, expiredUser, expired))
	if _, _, err := store.GetEnrollmentInvitationByTokenHash(ctx, "expired-hash", time.Now().UTC()); err != ErrNotFound {
		t.Fatalf("expected expired invite to be unavailable, got %v", err)
	}
}

func TestWebAuthnChallengeIsSingleUseAndExpires(t *testing.T) {
	store := newTestAuthStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	challenge := domain.WebAuthnChallenge{ID: "ch-1", Type: "login", Session: []byte(`{"challenge":"abc"}`), ExpiresAt: now.Add(time.Hour)}
	mustNoErr(t, store.CreateWebAuthnChallenge(ctx, challenge))
	if _, err := store.ConsumeWebAuthnChallenge(ctx, challenge.ID, challenge.Type, now); err != nil {
		t.Fatalf("consume challenge: %v", err)
	}
	if _, err := store.ConsumeWebAuthnChallenge(ctx, challenge.ID, challenge.Type, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected replay to fail, got %v", err)
	}

	expired := domain.WebAuthnChallenge{ID: "ch-2", Type: "login", Session: []byte(`{"challenge":"abc"}`), ExpiresAt: now.Add(-time.Minute)}
	mustNoErr(t, store.CreateWebAuthnChallenge(ctx, expired))
	if _, err := store.ConsumeWebAuthnChallenge(ctx, expired.ID, expired.Type, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected expired challenge to fail, got %v", err)
	}
}

func TestPasskeyCredentialLookupPreservesDisabledUser(t *testing.T) {
	store := newTestAuthStore(t)
	ctx := context.Background()
	user := domain.User{ID: "u-disabled", Email: "disabled@example.com", Role: domain.RoleEditor, Disabled: true}
	mustNoErr(t, store.CreateUser(ctx, user))
	mustNoErr(t, store.CreatePasskeyCredential(ctx, testPasskey("c-disabled", user.ID, []byte("credential-disabled"))))

	got, err := store.GetUserByCredentialID(ctx, []byte("credential-disabled"))
	if err != nil {
		t.Fatalf("lookup by credential: %v", err)
	}
	if !got.Disabled {
		t.Fatal("expected disabled flag to be preserved for passkey login rejection")
	}
}

func testPasskey(id, userID string, credentialID []byte) domain.PasskeyCredential {
	return domain.PasskeyCredential{
		ID:                 id,
		UserID:             userID,
		CredentialID:       credentialID,
		PublicKey:          []byte("public-key"),
		SignCount:          1,
		Nickname:           "Primary",
		WebAuthnCredential: []byte(`{"id":"Y3JlZGVudGlhbA","publicKey":"cHVibGljLWtleQ","authenticator":{"signCount":1}}`),
	}
}
