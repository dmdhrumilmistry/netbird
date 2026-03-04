package internal

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netbirdio/netbird/client/internal/peer"
)

func TestSessionWatcher_ExpiresSoonListenerCalled(t *testing.T) {
	statusRecorder := peer.NewRecorder("mgmt")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watcher := &SessionWatcher{
		ctx:                ctx,
		peerStatusRecorder: statusRecorder,
		watchTicker:        time.NewTicker(10 * time.Millisecond),
		warningThreshold:   sessionExpirationWarningThreshold,
	}

	// Simulate management connected so sendNotification is set
	statusRecorder.MarkManagementConnected()

	// Set login expires soon (within warning threshold)
	loginExpiresAt := time.Now().Add(5 * time.Minute)
	statusRecorder.SetLoginExpiresAt(loginExpiresAt)

	var calledWith time.Duration
	var called atomic.Bool
	watcher.SetOnExpiresSoonListener(func(remaining time.Duration) {
		calledWith = remaining
		called.Store(true)
	})

	go watcher.startWatcher()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if called.Load() {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if !called.Load() {
		t.Fatal("expected onExpiresSoonListener to be called, but it was not")
	}

	// remaining should be ≤ warning threshold and > 0
	if calledWith <= 0 || calledWith > sessionExpirationWarningThreshold {
		t.Errorf("unexpected remaining time: %v", calledWith)
	}
}

func TestSessionWatcher_ExpiresSoonListenerNotCalledWhenFarFromExpiry(t *testing.T) {
	statusRecorder := peer.NewRecorder("mgmt")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watcher := &SessionWatcher{
		ctx:                ctx,
		peerStatusRecorder: statusRecorder,
		watchTicker:        time.NewTicker(10 * time.Millisecond),
		warningThreshold:   sessionExpirationWarningThreshold,
	}

	// Simulate management connected
	statusRecorder.MarkManagementConnected()

	// Set login expiry far in the future (beyond warning threshold)
	statusRecorder.SetLoginExpiresAt(time.Now().Add(30 * time.Minute))

	var called atomic.Bool
	watcher.SetOnExpiresSoonListener(func(remaining time.Duration) {
		called.Store(true)
	})

	go watcher.startWatcher()

	time.Sleep(200 * time.Millisecond)

	if called.Load() {
		t.Fatal("expected onExpiresSoonListener NOT to be called when expiry is far away, but it was")
	}
}

func TestSessionWatcher_ExpiresSoonListenerNotCalledWhenNoExpiry(t *testing.T) {
	statusRecorder := peer.NewRecorder("mgmt")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watcher := &SessionWatcher{
		ctx:                ctx,
		peerStatusRecorder: statusRecorder,
		watchTicker:        time.NewTicker(10 * time.Millisecond),
		warningThreshold:   sessionExpirationWarningThreshold,
	}

	// Simulate management connected
	statusRecorder.MarkManagementConnected()

	// No login expiry set (zero value)
	statusRecorder.SetLoginExpiresAt(time.Time{})

	var called atomic.Bool
	watcher.SetOnExpiresSoonListener(func(remaining time.Duration) {
		called.Store(true)
	})

	go watcher.startWatcher()

	time.Sleep(200 * time.Millisecond)

	if called.Load() {
		t.Fatal("expected onExpiresSoonListener NOT to be called when no expiry is set, but it was")
	}
}

func TestSessionWatcher_ExpiresSoonListenerOnlyCalledOnce(t *testing.T) {
	statusRecorder := peer.NewRecorder("mgmt")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watcher := &SessionWatcher{
		ctx:                ctx,
		peerStatusRecorder: statusRecorder,
		watchTicker:        time.NewTicker(10 * time.Millisecond),
		warningThreshold:   sessionExpirationWarningThreshold,
	}

	// Simulate management connected
	statusRecorder.MarkManagementConnected()

	// Set login expiry soon
	statusRecorder.SetLoginExpiresAt(time.Now().Add(5 * time.Minute))

	var callCount atomic.Int32
	watcher.SetOnExpiresSoonListener(func(remaining time.Duration) {
		callCount.Add(1)
	})

	go watcher.startWatcher()

	// Wait long enough for multiple ticks
	time.Sleep(200 * time.Millisecond)

	if n := callCount.Load(); n != 1 {
		t.Errorf("expected onExpiresSoonListener to be called exactly once, but called %d times", n)
	}
}
