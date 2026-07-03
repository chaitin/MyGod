package realtime

import (
	"testing"
	"time"
)

func TestConnectionPoolTracksUserConnectionsAndSendsToAllConnections(t *testing.T) {
	pool := NewConnectionPool(Options{})
	first := NewConnection("conn-1", "user-1", 2, nil)
	second := NewConnection("conn-2", "user-1", 2, nil)
	other := NewConnection("conn-3", "user-2", 2, nil)

	if becameOnline := pool.Register(first); !becameOnline {
		t.Fatal("first Register() = false, want user to become online")
	}
	if becameOnline := pool.Register(second); becameOnline {
		t.Fatal("second Register() = true, want user to stay online")
	}
	pool.Register(other)

	if count := pool.Count("user-1"); count != 2 {
		t.Fatalf("Count(user-1) = %d, want 2", count)
	}
	if !pool.IsOnline("user-1") {
		t.Fatal("IsOnline(user-1) = false, want true")
	}

	message := NewEvent("test.event", map[string]any{
		"user_id": "user-2",
		"online":  true,
	})
	if sent := pool.SendToUser("user-1", message); sent != 2 {
		t.Fatalf("SendToUser() sent %d messages, want 2", sent)
	}

	for name, outgoing := range map[string]<-chan Envelope{
		"first":  first.Outgoing(),
		"second": second.Outgoing(),
	} {
		select {
		case got := <-outgoing:
			if got.Kind != KindEvent || got.Event != "test.event" {
				t.Fatalf("%s got %#v, want test.event event", name, got)
			}
		default:
			t.Fatalf("%s did not receive message", name)
		}
	}

	select {
	case got := <-other.Outgoing():
		t.Fatalf("other user received %#v, want no message", got)
	default:
	}

	if becameOffline := pool.Unregister(first); becameOffline {
		t.Fatal("Unregister(first) = true, want user to stay online")
	}
	if count := pool.Count("user-1"); count != 1 {
		t.Fatalf("Count(user-1) after first unregister = %d, want 1", count)
	}
	if becameOffline := pool.Unregister(second); !becameOffline {
		t.Fatal("Unregister(second) = false, want user to become offline")
	}
	if pool.IsOnline("user-1") {
		t.Fatal("IsOnline(user-1) = true, want false")
	}
}

func TestConnectionPoolRecordsPongWithPerUserThrottle(t *testing.T) {
	now := time.Date(2026, 7, 3, 1, 30, 0, 0, time.UTC)
	records := make([]time.Time, 0, 2)
	pool := NewConnectionPool(Options{
		LastOnlineUpdateInterval: time.Minute,
		Now: func() time.Time {
			return now
		},
		RecordUserPong: func(userID string, at time.Time) {
			if userID != "user-1" {
				t.Fatalf("RecordUserPong userID = %q, want user-1", userID)
			}
			records = append(records, at)
		},
	})

	pool.RecordPong("user-1")
	pool.RecordPong("user-1")
	now = now.Add(time.Minute)
	pool.RecordPong("user-1")

	if len(records) != 2 {
		t.Fatalf("record count = %d, want 2", len(records))
	}
	if !records[0].Equal(time.Date(2026, 7, 3, 1, 30, 0, 0, time.UTC)) {
		t.Fatalf("first record = %s, want initial time", records[0])
	}
	if !records[1].Equal(time.Date(2026, 7, 3, 1, 31, 0, 0, time.UTC)) {
		t.Fatalf("second record = %s, want throttled time", records[1])
	}
}

func TestConnectionPoolClosesAllConnectionsForUser(t *testing.T) {
	pool := NewConnectionPool(Options{})
	firstClosed := 0
	secondClosed := 0
	otherClosed := 0
	first := NewConnection("conn-1", "user-1", 2, func() {
		firstClosed += 1
	})
	second := NewConnection("conn-2", "user-1", 2, func() {
		secondClosed += 1
	})
	other := NewConnection("conn-3", "user-2", 2, func() {
		otherClosed += 1
	})
	pool.Register(first)
	pool.Register(second)
	pool.Register(other)

	if closed := pool.CloseUser("user-1"); closed != 2 {
		t.Fatalf("CloseUser() = %d, want 2", closed)
	}
	if firstClosed != 1 || secondClosed != 1 {
		t.Fatalf("closed counts = %d, %d, want 1, 1", firstClosed, secondClosed)
	}
	if otherClosed != 0 {
		t.Fatalf("other closed count = %d, want 0", otherClosed)
	}
	if count := pool.Count("user-1"); count != 0 {
		t.Fatalf("Count(user-1) = %d, want 0", count)
	}
	if count := pool.Count("user-2"); count != 1 {
		t.Fatalf("Count(user-2) = %d, want 1", count)
	}
}
