package outbox

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestNextFailureDelayBacksOffAndCaps(t *testing.T) {
	cfg := Config{FailureBase: 5 * time.Second, FailureCap: 5 * time.Minute}

	first := nextFailureDelay(1, cfg)
	if first < cfg.FailureBase || first > cfg.FailureBase+cfg.FailureBase/4 {
		t.Fatalf("first delay = %s, want base plus up to 25%% jitter", first)
	}

	// Doubling must be monotonic until the cap is reached.
	previous := time.Duration(0)
	for attempts := 1; attempts <= 12; attempts++ {
		delay := nextFailureDelay(attempts, cfg)
		if delay < previous {
			t.Fatalf("delay at attempts=%d = %s went below earlier %s", attempts, delay, previous)
		}
		previous = delay
	}
	if got := nextFailureDelay(12, cfg); got != cfg.FailureCap {
		t.Fatalf("delay at attempts=12 = %s, want exact cap %s", got, cfg.FailureCap)
	}
	if got := nextFailureDelay(50, cfg); got != cfg.FailureCap {
		t.Fatalf("delay at attempts=50 = %s, want exact cap %s", got, cfg.FailureCap)
	}
}

func TestNextFailureDelayClampsLowAttempts(t *testing.T) {
	cfg := Config{FailureBase: time.Second, FailureCap: time.Minute}
	if got := nextFailureDelay(0, cfg); got < time.Second {
		t.Fatalf("delay for attempts=0 = %s, want at least base", got)
	}
}

func TestIsFatalOutboxError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"undefined table", &pgconn.PgError{Code: "42P01"}, true},
		{"undefined column", &pgconn.PgError{Code: "42703"}, true},
		{"insufficient privilege", &pgconn.PgError{Code: "42501"}, true},
		{"invalid password", &pgconn.PgError{Code: "28P01"}, true},
		{"invalid catalog name", &pgconn.PgError{Code: "3D000"}, true},
		{"invalid schema name", &pgconn.PgError{Code: "3F000"}, true},
		{"connection failure", &pgconn.PgError{Code: "08006"}, false},
		{"unique violation", &pgconn.PgError{Code: "23505"}, false},
		{"plain error", errors.New("boom"), false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		if got := isFatalOutboxError(tc.err); got != tc.want {
			t.Fatalf("%s: isFatalOutboxError = %v, want %v", tc.name, got, tc.want)
		}
	}
}
