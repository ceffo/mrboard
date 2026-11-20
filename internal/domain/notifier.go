package domain

import "context"

// Notifier sends a notification about an MR event.
// Implementations live in internal/adapters/ and are wired by the composition root.
type Notifier interface {
	// Notify announces mr and its current approvers.
	Notify(ctx context.Context, mr MergeRequest) error
	// NotifyApproverChange announces who became and who stopped being a
	// required approver of mr, and who changed the rule.
	NotifyApproverChange(ctx context.Context, mr MergeRequest, change ApproverChange) error
}
