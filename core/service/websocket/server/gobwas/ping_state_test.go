package gobwas

import (
	"testing"
	"time"
)

func TestGobwasConnectionNextPingAction(t *testing.T) {
	now := time.Now()
	conn := &GobwasConnection{
		lastReadAt: now.Add(-20 * time.Second),
	}

	action := conn.nextPingAction(now, 15*time.Second, 8*time.Second)
	if action != pingActionSend {
		t.Fatalf("expected pingActionSend, got %v", action)
	}

	if !conn.awaitingPong {
		t.Fatal("expected awaitingPong to be set after sending ping")
	}

	action = conn.nextPingAction(now.Add(2*time.Second), 15*time.Second, 8*time.Second)
	if action != pingActionSkip {
		t.Fatalf("expected pingActionSkip while waiting for pong, got %v", action)
	}

	action = conn.nextPingAction(now.Add(9*time.Second), 15*time.Second, 8*time.Second)
	if action != pingActionClose {
		t.Fatalf("expected pingActionClose after pong timeout, got %v", action)
	}
}

func TestGobwasConnectionNotePongClearsAwaitingState(t *testing.T) {
	now := time.Now()
	conn := &GobwasConnection{
		lastReadAt:   now.Add(-20 * time.Second),
		lastPingAt:   now.Add(-2 * time.Second),
		awaitingPong: true,
	}

	conn.notePong(now)

	if conn.awaitingPong {
		t.Fatal("expected awaitingPong to be cleared on pong")
	}
	if !conn.lastPongAt.Equal(now) {
		t.Fatalf("expected lastPongAt to be updated, got %v want %v", conn.lastPongAt, now)
	}
	if !conn.lastReadAt.Equal(now) {
		t.Fatalf("expected lastReadAt to be updated, got %v want %v", conn.lastReadAt, now)
	}
}
