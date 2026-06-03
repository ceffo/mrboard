package domain

import "context"

// Notifier sends a notification about an MR event.
// Implementations live in internal/adapters/ and are wired by the composition root.
type Notifier interface {
	Notify(ctx context.Context, mr MergeRequest) error
}
