package recurrences_test

import (
	"context"
	"testing"

	"github.com/greg0x46/julius/internal/recurrences"
)

// Overrides are keyed by occurrence date, so a schedule change must drop the
// ones whose date is no longer an occurrence, and free their transactions.
func TestUpdateDropsOverridesOfDatesNoLongerOccurrences(t *testing.T) {
	conn := newTestDB(t)
	ctx := context.Background()
	commitment := newCommitment(t, conn)
	linkedTx := seedTransaction(t, conn, "tx-1")
	keptTx := seedTransaction(t, conn, "tx-2")

	if _, err := recurrences.PutOverride(ctx, conn, commitment.ID, date(t, "2026-02-05"), recurrences.StateLinked, &linkedTx); err != nil {
		t.Fatalf("PutOverride: %v", err)
	}
	if _, err := recurrences.PutOverride(ctx, conn, commitment.ID, date(t, "2026-03-05"), recurrences.StateDetached, nil); err != nil {
		t.Fatalf("PutOverride: %v", err)
	}
	if _, err := recurrences.PutOverride(ctx, conn, commitment.ID, date(t, "2026-04-10"), recurrences.StateLinked, &keptTx); err != nil {
		t.Fatalf("PutOverride: %v", err)
	}

	write := testWrite(t, commitment.CategoryID)
	write.DayOfMonth = 10
	if _, err := recurrences.Update(ctx, conn, commitment.ID, write); err != nil {
		t.Fatalf("Update: %v", err)
	}

	for _, orphan := range []string{"2026-02-05", "2026-03-05"} {
		if _, found, err := recurrences.GetOverride(ctx, conn, commitment.ID, date(t, orphan)); err != nil || found {
			t.Errorf("override of %s: found=%v err=%v, want dropped", orphan, found, err)
		}
	}
	if _, found, err := recurrences.GetOverride(ctx, conn, commitment.ID, date(t, "2026-04-10")); err != nil || !found {
		t.Errorf("override on a surviving occurrence: found=%v err=%v, want kept", found, err)
	}
	linked, err := recurrences.LinkedTransactionIDs(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}
	if linked[linkedTx] || !linked[keptTx] {
		t.Errorf("linked = %v, want only the surviving occurrence's transaction", linked)
	}
}
