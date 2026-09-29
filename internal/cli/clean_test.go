package cli

import (
	"testing"
	"time"
)

func TestParseAge(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"90d", 90 * day, false},
		{"12w", 12 * week, false},
		{"720h", 720 * time.Hour, false},
		{"0d", 0, true},
		{"-5d", 0, true},
		{"soon", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseAge(tt.in)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("parseAge(%q) = %v, %v; want %v, error %v", tt.in, got, err, tt.want, tt.wantErr)
			}
		})
	}
}
