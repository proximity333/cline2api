package main

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func TestIsRetryableTransportError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"deadline exceeded", context.DeadlineExceeded, true},
		{"eof", io.EOF, true},
		{"unexpected eof", io.ErrUnexpectedEOF, true},
		{"plain error", errors.New("connection reset"), false},
	}
	for _, tt := range tests {
		if got := isRetryableTransportError(tt.err); got != tt.want {
			t.Errorf("%s: isRetryableTransportError = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestParseCooldownUntil(t *testing.T) {
	tests := []struct {
		name string
		body string
		want time.Duration
	}{
		{"json 1h", `{"error":"quota","message":"Try again in 1h"}`, time.Hour},
		{"minutes only", "Try again in 5m", 5 * time.Minute},
		{"minutes word", "Try again in 5 minutes", 5 * time.Minute},
		{"30m", "Try again in 30m", 30 * time.Minute},
		{"one hour word", "Try again in 1 hour", time.Hour},
		{"hours and minutes", "Try again in 1h 1m", time.Hour + time.Minute},
		{"words", "Try again in 2 hours 15 mins", 2*time.Hour + 15*time.Minute},
		{"seconds", "Try again in 90 seconds", 90 * time.Second},
		{"compact hours minutes", "Try again in 1h30m", time.Hour + 30*time.Minute},
		{"no hint falls back", "rate limited", time.Hour},
	}
	for _, tt := range tests {
		got := time.Until(parseCooldownUntil(tt.body))
		if diff := got - tt.want; diff < -3*time.Second || diff > 3*time.Second {
			t.Errorf("%s: parseCooldownUntil = %s, want ~%s", tt.name, got, tt.want)
		}
	}
}

func TestCoolDownAccountEscalatesThenCaps(t *testing.T) {
	oldPool := pool
	t.Cleanup(func() { pool = oldPool })

	acc := &Account{AccountID: "backoff", Email: "backoff@example.com", Status: "active"}
	pool = &AccountPool{Accounts: []*Account{acc}}

	want := []time.Duration{
		30 * time.Second,
		time.Minute,
		5 * time.Minute,
		15 * time.Minute,
		30 * time.Minute,
		30 * time.Minute, // 超过序列长度后保持上限
	}
	for i, d := range want {
		before := time.Now()
		coolDownAccount(acc, errors.New("boom"))
		got := acc.CooldownUntil.Sub(before)
		if got < d || got > d+time.Second {
			t.Fatalf("cooldown %d = %s, want ~%s", i, got, d)
		}
		if acc.CooldownCount != i+1 {
			t.Fatalf("cooldown count = %d, want %d", acc.CooldownCount, i+1)
		}
		if acc.Status != "cooldown" {
			t.Fatalf("status = %q, want cooldown", acc.Status)
		}
	}
}
