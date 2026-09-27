package timing

import (
	"context"
	"testing"
	"time"
)

func TestWithReceivedRoundTrip(t *testing.T) {
	want := time.Date(2026, 9, 26, 14, 30, 0, 123456789, time.UTC)
	ctx := WithReceived(context.Background(), want)

	got, ok := Received(ctx)
	if !ok {
		t.Fatal("Received reported no stamp")
	}
	if !got.Equal(want) {
		t.Fatalf("Received = %v, want %v", got, want)
	}
	if got.Nanosecond() != want.Nanosecond() {
		t.Fatalf("nanoseconds lost: %d", got.Nanosecond())
	}
}

func TestReceivedAbsent(t *testing.T) {
	if _, ok := Received(context.Background()); ok {
		t.Fatal("a plain context must carry no stamp")
	}
	if _, ok := Received(nil); ok {
		t.Fatal("a nil context must carry no stamp")
	}
}

func TestWithReceivedToleratesNilContext(t *testing.T) {
	ctx := WithReceived(nil, time.Now())
	if _, ok := Received(ctx); !ok {
		t.Fatal("WithReceived(nil) must still produce a usable context")
	}
}

func TestReceivedSurvivesChildContext(t *testing.T) {
	want := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	parent := WithReceived(context.Background(), want)

	child, cancel := context.WithCancel(parent)
	defer cancel()

	got, ok := Received(child)
	if !ok || !got.Equal(want) {
		t.Fatalf("child Received = %v, %v; want %v, true", got, ok, want)
	}
}
