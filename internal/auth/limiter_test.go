package auth

import (
	"fmt"
	"testing"
	"time"
)

type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time { return c.now }

func newTestLimiter(perKey, total int) (*Limiter, *testClock) {
	clock := &testClock{now: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)}
	l := NewLimiter(perKey, total)
	l.Now = clock.Now
	return l, clock
}

func TestLimiterBlockedChecksWithoutCounting(t *testing.T) {
	l, _ := newTestLimiter(5, 30)
	for i := 0; i < 100; i++ {
		if l.Blocked("a") {
			t.Fatalf("blocked after %d checks and no recorded event", i)
		}
	}
	for i := 0; i < 4; i++ {
		l.Record("a")
	}
	if l.Blocked("a") {
		t.Fatal("blocked below the per-key budget")
	}
	l.Record("a")
	if !l.Blocked("a") {
		t.Fatal("not blocked at the per-key budget")
	}
	if l.Blocked("b") {
		t.Fatal("another key shares the per-key budget")
	}
}

func TestLimiterOverallBudgetBacksUpPerKey(t *testing.T) {
	l, _ := newTestLimiter(5, 30)
	for i := 0; i < 29; i++ {
		l.Record(fmt.Sprintf("key-%d", i))
	}
	if l.Blocked("fresh") {
		t.Fatal("blocked below the overall budget")
	}
	l.Record("last")
	if !l.Blocked("fresh") {
		t.Fatal("overall budget not enforced for a new key")
	}
}

func TestLimiterWindowRollsOver(t *testing.T) {
	l, clock := newTestLimiter(2, 10)
	l.Record("a")
	l.Record("a")
	clock.now = clock.now.Add(59 * time.Second)
	if !l.Blocked("a") {
		t.Fatal("window ended early")
	}
	clock.now = clock.now.Add(time.Second)
	if l.Blocked("a") {
		t.Fatal("budget survived the window")
	}
	l.Record("a")
	if l.Blocked("a") {
		t.Fatal("the new window should start empty")
	}
}

func TestLimiterTakeCountsOnlyWhatFits(t *testing.T) {
	l, clock := newTestLimiter(0, 3)
	for i := 0; i < 3; i++ {
		if !l.Take("") {
			t.Fatalf("take %d refused", i)
		}
	}
	for i := 0; i < 5; i++ {
		if l.Take("") {
			t.Fatal("take past the budget")
		}
	}
	clock.now = clock.now.Add(time.Minute)
	if !l.Take("") {
		t.Fatal("budget not restored")
	}
}

func TestLimiterBoundsItsKeys(t *testing.T) {
	l, _ := newTestLimiter(1, 0)
	for i := 0; i < maxLimiterKeys+100; i++ {
		l.Record(fmt.Sprintf("key-%d", i))
	}
	if len(l.keys) > maxLimiterKeys {
		t.Fatalf("%d keys tracked, want at most %d", len(l.keys), maxLimiterKeys)
	}
	if !l.Blocked("key-0") {
		t.Fatal("a tracked key lost its count")
	}
}

func TestZeroBudgetsDisableTheLimit(t *testing.T) {
	var l Limiter
	for i := 0; i < 1000; i++ {
		l.Record("a")
		if l.Blocked("a") {
			t.Fatal("a limiter without budgets blocked")
		}
	}
}
