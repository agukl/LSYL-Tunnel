package tunnel

import "testing"

func TestRuntimeEventRingPreservesInsertionOrderBeforeCapacity(t *testing.T) {
	ring := newRuntimeEventRing(3)
	ring.Append(RuntimeEvent{RequestID: "one"})
	ring.Append(RuntimeEvent{RequestID: "two"})

	assertRuntimeEventIDs(t, ring.Snapshot(), "one", "two")
}

func TestRuntimeEventRingReturnsOldestToNewestAfterWrap(t *testing.T) {
	ring := newRuntimeEventRing(3)
	for _, id := range []string{"one", "two", "three", "four", "five"} {
		ring.Append(RuntimeEvent{RequestID: id})
	}

	assertRuntimeEventIDs(t, ring.Snapshot(), "three", "four", "five")
}

func TestRuntimeEventRingCapacityOneAndSnapshotIsolation(t *testing.T) {
	ring := newRuntimeEventRing(1)
	ring.Append(RuntimeEvent{RequestID: "one"})
	ring.Append(RuntimeEvent{RequestID: "two"})

	snapshot := ring.Snapshot()
	assertRuntimeEventIDs(t, snapshot, "two")
	snapshot[0].RequestID = "changed"
	assertRuntimeEventIDs(t, ring.Snapshot(), "two")
}

func assertRuntimeEventIDs(t *testing.T, events []RuntimeEvent, want ...string) {
	t.Helper()
	if len(events) != len(want) {
		t.Fatalf("event count = %d, want %d: %#v", len(events), len(want), events)
	}
	for i := range want {
		if events[i].RequestID != want[i] {
			t.Fatalf("event %d request ID = %q, want %q", i, events[i].RequestID, want[i])
		}
	}
}
