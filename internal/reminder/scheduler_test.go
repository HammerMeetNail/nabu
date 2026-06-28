package reminder

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeLeaderLock struct {
	acquired bool
	err      error
}

func (f fakeLeaderLock) TryAcquire(context.Context) (bool, error) { return f.acquired, f.err }
func (f fakeLeaderLock) Release(context.Context) error            { return nil }

func TestAcquireLeadership(t *testing.T) {
	tests := []struct {
		name   string
		leader LeaderLock
		want   bool
	}{
		{"no lock means always leader", nil, true},
		{"lock acquired", fakeLeaderLock{acquired: true}, true},
		{"lock held by another instance", fakeLeaderLock{acquired: false}, false},
		{"acquire error means not leader", fakeLeaderLock{err: errors.New("boom")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Scheduler{leader: tt.leader}
			was := false
			if got := s.acquireLeadership(context.Background(), &was); got != tt.want {
				t.Fatalf("acquireLeadership() = %v, want %v", got, tt.want)
			}
			// wasLeader is only tracked for transition logging when a lock is set.
			if tt.leader != nil && was != tt.want {
				t.Fatalf("wasLeader = %v, want %v", was, tt.want)
			}
		})
	}
}

func TestParseHM(t *testing.T) {
	tests := []struct {
		input   string
		wantH   int
		wantM   int
		wantErr bool
	}{
		{"14:30", 14, 30, false},
		{"00:00", 0, 0, false},
		{"9:05", 9, 5, false},
		{"invalid", 0, 0, true},
		{"", 0, 0, true},
		{"14:", 0, 0, true},
	}

	for _, tt := range tests {
		h, m, err := parseHM(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseHM(%q) error = %v, wantErr = %v", tt.input, err, tt.wantErr)
			continue
		}
		if err != nil {
			continue
		}
		if h != tt.wantH || m != tt.wantM {
			t.Errorf("parseHM(%q) = (%d, %d), want (%d, %d)", tt.input, h, m, tt.wantH, tt.wantM)
		}
	}
}

func TestFormatTime(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"08:00", "8:00 AM"},
		{"14:30", "2:30 PM"},
		{"00:00", "12:00 AM"},
		{"12:00", "12:00 PM"},
		{"09:05", "9:05 AM"},
	}

	for _, tt := range tests {
		got := formatTime(tt.input)
		if got != tt.want {
			t.Errorf("formatTime(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestComputeRemindTime(t *testing.T) {
	now := time.Date(2025, 6, 7, 8, 0, 0, 0, time.UTC)

	tests := []struct {
		specificTime string
		leadMinutes  int
		wantHour     int
		wantMin      int
	}{
		{"14:00", 10, 13, 50},
		{"08:00", 5, 7, 55},
		{"00:00", 60, 23, 0}, // midnight - 60 min = 11 PM previous day? let's compute...
		{"14:30", 15, 14, 15},
	}

	for _, tt := range tests {
		got := computeRemindTime(now, tt.specificTime, tt.leadMinutes)
		if got.Hour() != tt.wantHour || got.Minute() != tt.wantMin {
			t.Errorf("computeRemindTime(now, %q, %d) = %s, want hour=%d min=%d",
				tt.specificTime, tt.leadMinutes, got.Format("15:04"), tt.wantHour, tt.wantMin)
		}
	}
}

func TestIsBetween(t *testing.T) {
	tests := []struct {
		time   string
		start  string
		end    string
		wantIn bool
	}{
		{"10:00", "08:00", "12:00", true},
		{"06:00", "08:00", "12:00", false},
		{"14:00", "08:00", "12:00", false},
		{"22:00", "20:00", "06:00", true},  // overnight
		{"03:00", "20:00", "06:00", true},  // overnight
		{"10:00", "20:00", "06:00", false}, // overnight gap
	}

	for _, tt := range tests {
		tm, _ := time.Parse("15:04", tt.time)
		got := isBetween(tm, tt.start, tt.end)
		if got != tt.wantIn {
			t.Errorf("isBetween(%s, %q, %q) = %v, want %v", tt.time, tt.start, tt.end, got, tt.wantIn)
		}
	}
}
