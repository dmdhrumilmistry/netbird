package internal

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/netbirdio/netbird/client/internal/peer"
)

const (
	sessionExpirationWarningThreshold    = 10 * time.Minute
	sessionExpirationWarningThresholdVar = "NB_SESSION_EXPIRY_WARNING_THRESHOLD"
)

type SessionWatcher struct {
	ctx   context.Context
	mutex sync.Mutex

	peerStatusRecorder *peer.Status
	watchTicker        *time.Ticker

	sendNotification      bool
	onExpireListener      func()
	onExpiresSoonListener func(remainingTime time.Duration)
	expiresSoonNotified   bool
	warningThreshold      time.Duration
}

// NewSessionWatcher creates a new instance of SessionWatcher.
// The warning threshold defaults to sessionExpirationWarningThreshold but can be
// overridden at startup via the NB_SESSION_EXPIRY_WARNING_THRESHOLD environment variable
// (e.g. "5m", "15m30s").
func NewSessionWatcher(ctx context.Context, peerStatusRecorder *peer.Status) *SessionWatcher {
	threshold := sessionExpirationWarningThreshold
	if v := os.Getenv(sessionExpirationWarningThresholdVar); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			threshold = d
		} else {
			log.Warnf("unable to parse %s: %s, using default: %s", sessionExpirationWarningThresholdVar, v, threshold)
		}
	}

	s := &SessionWatcher{
		ctx:                ctx,
		peerStatusRecorder: peerStatusRecorder,
		watchTicker:        time.NewTicker(2 * time.Second),
		warningThreshold:   threshold,
	}
	go s.startWatcher()
	return s
}

// SetWarningThreshold sets how far in advance of session expiry the warning notification fires.
func (s *SessionWatcher) SetWarningThreshold(d time.Duration) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.warningThreshold = d
}

// SetOnExpireListener sets the callback func to be called when the session expires.
func (s *SessionWatcher) SetOnExpireListener(onExpire func()) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.onExpireListener = onExpire
}

// SetOnExpiresSoonListener sets the callback func to be called when the session is about to expire.
// The callback receives the remaining time until expiration.
func (s *SessionWatcher) SetOnExpiresSoonListener(onExpiresSoon func(remainingTime time.Duration)) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.onExpiresSoonListener = onExpiresSoon
}

// startWatcher continuously checks if the session requires login and
// calls the onExpireListener if login is required.
func (s *SessionWatcher) startWatcher() {
	for {
		select {
		case <-s.ctx.Done():
			s.watchTicker.Stop()
			return
		case <-s.watchTicker.C:
			managementState := s.peerStatusRecorder.GetManagementState()
			if managementState.Connected {
				s.sendNotification = true
				s.checkLoginExpiresSoon()
			}

			isLoginRequired := s.peerStatusRecorder.IsLoginRequired()
			if isLoginRequired && s.sendNotification && s.onExpireListener != nil {
				s.mutex.Lock()
				s.onExpireListener()
				s.sendNotification = false
				s.expiresSoonNotified = false
				s.mutex.Unlock()
			}
		}
	}
}

// checkLoginExpiresSoon checks if the login is about to expire and fires the onExpiresSoonListener
// when the remaining time falls below the warning threshold.
func (s *SessionWatcher) checkLoginExpiresSoon() {
	loginExpiresAt := s.peerStatusRecorder.GetLoginExpiresAt()
	if loginExpiresAt.IsZero() {
		return
	}

	remaining := time.Until(loginExpiresAt)
	if remaining <= 0 {
		// Already expired; the expiry listener will handle this
		return
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	if remaining <= s.warningThreshold && !s.expiresSoonNotified && s.onExpiresSoonListener != nil {
		s.onExpiresSoonListener(remaining)
		s.expiresSoonNotified = true
	}
}

// CheckUIApp checks whether UI application is running.
func CheckUIApp() bool {
	cmd := exec.Command("ps", "-ef")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "netbird-ui") && !strings.Contains(line, "grep") {
			return true
		}
	}
	return false
}
