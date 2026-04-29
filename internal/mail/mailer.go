package mail

import "context"

type Mailer interface {
	SendPasswordReset(ctx context.Context, to, resetURL string) error
	SendLaunchNotification(ctx context.Context, to, appBaseURL string) error
	SendCampaign(ctx context.Context, to, subject, body string) error
	SendNotification(ctx context.Context, to, subject, body string) error
}
