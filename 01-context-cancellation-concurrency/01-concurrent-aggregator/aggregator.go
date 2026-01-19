package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"
	
	"golang.org/x/sync/errgroup"
)

// --- Types & Constants ---

type UserData struct {
	Profile string
	Orders  string
}

type UserAggregator struct {
	timeout time.Duration
	logger  *slog.Logger
	profileFunc func(context.Context, int) (string, error)
	orderFunc func(context.Context, int) (string, error)
}

// TODO: Define Option type for Functional Options

type Option func(*UserAggregator)

func WithTimeout(timeout time.Duration) Option {
	return func(ua *UserAggregator) {
		ua.timeout = timeout
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(ua *UserAggregator) {
		ua.logger = logger
	}
}

// --- Mock Services ---

func fetchProfile(ctx context.Context, id int) (string, error) {
	// Simulate work
	select {
	case <-time.After(500 * time.Millisecond):
		return "Alice", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func fetchOrders(ctx context.Context, id int) (string, error) {
	// Simulate work
	select {
	case <-time.After(700 * time.Millisecond):
		return "5", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// --- Implementation ---

// New initializes the aggregator with Functional Options.
func New(opts ...Option) *UserAggregator {
	// Default values
	agg := &UserAggregator{
		timeout: 2 * time.Second,
		logger:  slog.Default(),
		profileFunc: fetchProfile,
		orderFunc: fetchOrders,
	}

	// TODO: Apply options
    for _, opt := range opts {
		opt(agg)
	}
	return agg
}

// TODO: Implement WithTimeout and WithLogger options

func (ua *UserAggregator) Aggregate(ctx context.Context, userID int) (string, error) {
	// 1. Create a derived context with the aggregator's timeout
	// 2. Create an errgroup.WithContext(ctx)
	// 3. Launch fetchProfile and fetchOrders in g.Go()
	// 4. Wait for results and handle the error propagation
	ctx, cancel := context.WithTimeout(ctx, ua.timeout)
	defer cancel()	
	
	g, gCtx := errgroup.WithContext(ctx)

	var profileName string
	var orderCount string	
	g.Go(func() error {
		p, err := ua.profileFunc(gCtx, userID)
		if err != nil {
			return fmt.Errorf("fetchProfile failed: %w", err)
		}
		profileName = p
		return nil
	})

	g.Go(func() error {
		o, err := ua.orderFunc(gCtx, userID)
		if err != nil {
			return fmt.Errorf("fetchOrders failed: %w", err)
		}
		orderCount = o
		return nil
	})

	if err := g.Wait(); err != nil {
		return "", err
	}

	return fmt.Sprintf("Profile: %s, Orders: %s", profileName, orderCount), nil
}

func main() {
	// Example usage
	agg := New()
	
	ctx := context.Background()
	result, err := agg.Aggregate(ctx, 1)
	if err != nil {
		slog.Error("Aggregation failed", "error", err)
		return
	}

	fmt.Println("Final Output:", result)
}