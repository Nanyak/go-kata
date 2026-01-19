package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestUserAggregator_Aggregate_TableDriven(t *testing.T) {
	tests := []struct {
		name           string
		timeout        time.Duration
		// We add these so we can inject specific behaviors per test
		mockProfile    func(context.Context, int) (string, error)
		mockOrders     func(context.Context, int) (string, error)
		wantErrContain string
	}{
		{
			name:           "Success Case",
			timeout:        1 * time.Second,
			wantErrContain: "",
			// mockProfile and mockOrders will be nil, so we use defaults in the loop
		},
		{
			name:           "Timeout Case (Slow Poke)",
			timeout:        100 * time.Millisecond,
			wantErrContain: context.DeadlineExceeded.Error(),
		},
		{
			name:    "Domino Effect (Instant Failure)",
			timeout: 2 * time.Second, // Long timeout so the service error hits first
			mockProfile: func(ctx context.Context, id int) (string, error) {
				return "", errors.New("profile service exploded")
			},
			mockOrders: func(ctx context.Context, id int) (string, error) {
				select {
				case <-time.After(1 * time.Second): // Should be cancelled way before this
					return "5", nil
				case <-ctx.Done():
					return "", ctx.Err()
				}
			},
			wantErrContain: "profile service exploded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agg := New(WithTimeout(tt.timeout))

			// Inject mocks if they are defined in the table
			if tt.mockProfile != nil {
				agg.profileFunc = tt.mockProfile
			}
			if tt.mockOrders != nil {
				agg.orderFunc = tt.mockOrders
			}

			start := time.Now()
			_, err := agg.Aggregate(context.Background(), 1)
			duration := time.Since(start)

			// Verify Error
			if tt.wantErrContain != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrContain) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErrContain, err)
				}
			} else if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			// Verify Fail-Fast Logic for the Domino Case
			if tt.name == "Domino Effect (Instant Failure)" {
				if duration > 100*time.Millisecond {
					t.Errorf("Domino effect failed: took %v, expected near-instant return", duration)
				}
			}
		})
	}
}