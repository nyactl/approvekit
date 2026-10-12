package approval

import (
	"testing"
	"time"
)

func TestStateIsSingleUseAndBound(t *testing.T) {
	t.Parallel()
	g := NewGate()
	now := time.Now()
	g.now = func() time.Time { return now }
	s := g.issue("a")
	if g.redeem(s, "b") {
		t.Error("redeemed for another digest")
	}
	s = g.issue("a")
	if !g.redeem(s, "a") || g.redeem(s, "a") {
		t.Error("state must redeem exactly once")
	}
	s = g.issue("a")
	now = now.Add(TTL + time.Second)
	if g.redeem(s, "a") {
		t.Error("expired state redeemed")
	}
	if g.redeem("00.ff", "a") || g.redeem("no-dot", "a") {
		t.Error("forged state redeemed")
	}
}
