package mail

import "context"

type Mailer interface {
	SendPasswordReset(ctx context.Context, to, resetURL string) error
}
