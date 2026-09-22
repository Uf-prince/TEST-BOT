package main

import (
	"context"
	"testing"
	"time"
)

// waitCancelled returns true if ctx is cancelled within d.
func waitCancelled(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return true
	case <-time.After(d):
		return false
	}
}

// TestCmdGuard_SameUserSameSession_LatestWins verifies the core requirement:
// a NEW command from the SAME user on the SAME session cancels the PREVIOUS
// in-flight command instantly.
func TestCmdGuard_SameUserSameSession_LatestWins(t *testing.T) {
	session := "111@s.whatsapp.net"
	user := "999@s.whatsapp.net"

	ctx1, release1 := BeginCmdGuard(session, user)
	defer release1()

	// First command is running (not cancelled yet).
	if waitCancelled(ctx1, 50*time.Millisecond) {
		t.Fatal("first command was cancelled before a newer one arrived")
	}

	// Same user sends a NEW command on the SAME session.
	ctx2, release2 := BeginCmdGuard(session, user)
	defer release2()

	// The FIRST command must be cancelled now (latest wins).
	if !waitCancelled(ctx1, 200*time.Millisecond) {
		t.Fatal("previous command of same user/session was NOT cancelled")
	}
	// The NEW command must still be alive.
	if waitCancelled(ctx2, 50*time.Millisecond) {
		t.Fatal("new command was cancelled unexpectedly")
	}
}

// TestCmdGuard_DifferentUserSameSession_Isolated verifies that a new command
// from user 1 does NOT cancel user 2's in-flight command on the SAME session.
func TestCmdGuard_DifferentUserSameSession_Isolated(t *testing.T) {
	session := "111@s.whatsapp.net"
	user1 := "user1@s.whatsapp.net"
	user2 := "user2@s.whatsapp.net"

	// user2 starts a long download.
	ctx2, release2 := BeginCmdGuard(session, user2)
	defer release2()

	// user1 starts a command on the SAME session.
	ctx1, release1 := BeginCmdGuard(session, user1)
	defer release1()

	// user1's new command must NOT cancel user2's command.
	if waitCancelled(ctx2, 200*time.Millisecond) {
		t.Fatal("user2's command was cancelled by user1's command on same session")
	}
	// user1's own command is alive.
	if waitCancelled(ctx1, 50*time.Millisecond) {
		t.Fatal("user1's command was cancelled unexpectedly")
	}

	// Now user1 sends ANOTHER command — only user1's previous is cancelled.
	ctx1b, release1b := BeginCmdGuard(session, user1)
	defer release1b()
	if !waitCancelled(ctx1, 200*time.Millisecond) {
		t.Fatal("user1's previous command was NOT cancelled by user1's newer command")
	}
	if waitCancelled(ctx2, 50*time.Millisecond) {
		t.Fatal("user2's command was cancelled by user1's newer command")
	}
	if waitCancelled(ctx1b, 50*time.Millisecond) {
		t.Fatal("user1's newest command was cancelled unexpectedly")
	}
}

// TestCmdGuard_DifferentSession_Isolated verifies that a command on session A
// never affects a command on session B (bot 1 vs bot 2), even for the SAME user.
func TestCmdGuard_DifferentSession_Isolated(t *testing.T) {
	sessionA := "botA@s.whatsapp.net"
	sessionB := "botB@s.whatsapp.net"
	user := "sameuser@s.whatsapp.net"

	// Bot A: user starts a download.
	ctxA, releaseA := BeginCmdGuard(sessionA, user)
	defer releaseA()

	// Bot B: SAME user starts a download.
	ctxB, releaseB := BeginCmdGuard(sessionB, user)
	defer releaseB()

	// Neither should cancel the other — different sessions.
	if waitCancelled(ctxA, 200*time.Millisecond) {
		t.Fatal("session A command was cancelled by session B command")
	}
	if waitCancelled(ctxB, 200*time.Millisecond) {
		t.Fatal("session B command was cancelled by session A command")
	}

	// A newer command on session A must cancel ONLY session A's previous.
	ctxA2, releaseA2 := BeginCmdGuard(sessionA, user)
	defer releaseA2()
	if !waitCancelled(ctxA, 200*time.Millisecond) {
		t.Fatal("session A previous command was NOT cancelled by session A newer command")
	}
	if waitCancelled(ctxB, 50*time.Millisecond) {
		t.Fatal("session B command was cancelled by session A newer command")
	}
	if waitCancelled(ctxA2, 50*time.Millisecond) {
		t.Fatal("session A newest command was cancelled unexpectedly")
	}
}

// TestCmdGuard_ReleaseCleansUp verifies that release() removes the entry so
// the maps never accumulate dead entries (0% memory growth over time).
func TestCmdGuard_ReleaseCleansUp(t *testing.T) {
	session := "cleanup@s.whatsapp.net"
	user := "u@s.whatsapp.net"

	_, release := BeginCmdGuard(session, user)
	g := guardForSession(session)
	g.mu.Lock()
	n := len(g.entries)
	g.mu.Unlock()
	if n != 1 {
		t.Fatalf("expected 1 live entry, got %d", n)
	}

	release()

	g.mu.Lock()
	n = len(g.entries)
	g.mu.Unlock()
	if n != 0 {
		t.Fatalf("expected 0 live entries after release, got %d", n)
	}
}

// TestCmdGuard_StaleReleaseDoesNotKillNewer verifies that when an OLD command
// finishes AFTER a newer one replaced it, its release() does NOT delete or
// cancel the newer command's entry.
func TestCmdGuard_StaleReleaseDoesNotKillNewer(t *testing.T) {
	session := "stale@s.whatsapp.net"
	user := "u@s.whatsapp.net"

	_, release1 := BeginCmdGuard(session, user)
	ctx2, release2 := BeginCmdGuard(session, user)
	defer release2()

	// Old command finishes late.
	release1()

	// Newer command must still be alive and still registered.
	if waitCancelled(ctx2, 50*time.Millisecond) {
		t.Fatal("newer command was cancelled by stale release")
	}
	g := guardForSession(session)
	g.mu.Lock()
	n := len(g.entries)
	g.mu.Unlock()
	if n != 1 {
		t.Fatalf("expected newer entry to survive stale release, got %d entries", n)
	}
}

// TestCmdGuard_DropSession verifies dropSessionGuard removes a session's guard.
func TestCmdGuard_DropSession(t *testing.T) {
	session := "dropme@s.whatsapp.net"
	_, release := BeginCmdGuard(session, "u@s.whatsapp.net")
	defer release()

	if _, ok := sessionGuards.Load(session); !ok {
		t.Fatal("session guard should exist before drop")
	}
	dropSessionGuard(session)
	if _, ok := sessionGuards.Load(session); ok {
		t.Fatal("session guard should be gone after drop")
	}
}
