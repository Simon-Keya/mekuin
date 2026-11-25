package main

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type queuedEmail struct {
	ID, Recipient, Subject, Body string
	Attempts                     int
}

// runEmailOutboxWorker delivers only committed messages. A crash after claim is
// recoverable: claims older than five minutes are eligible for another worker.
func (s *server) runEmailOutboxWorker(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		s.deliverEmailBatch(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *server) deliverEmailBatch(ctx context.Context) {
	for i := 0; i < 10; i++ {
		var m queuedEmail
		err := s.db.QueryRow(ctx, `SELECT id,recipient,subject,body,attempts FROM claim_email_outbox()`).Scan(&m.ID, &m.Recipient, &m.Subject, &m.Body, &m.Attempts)
		if err == pgx.ErrNoRows {
			return
		}
		if err != nil {
			s.log.Error("email outbox claim failed", "error", err)
			return
		}
		err = sendOutboxEmail(m.Recipient, m.Subject, m.Body)
		if err == nil {
			_, err = s.db.Exec(ctx, `SELECT complete_email_outbox($1,true,'',1)`, m.ID)
		} else {
			delay := time.Duration(1<<min(m.Attempts, 8)) * time.Minute
			_, updateErr := s.db.Exec(ctx, `SELECT complete_email_outbox($1,false,$2,$3)`, m.ID, err.Error(), int64(delay.Seconds()))
			if updateErr != nil {
				s.log.Error("email outbox retry update failed", "error", updateErr)
			}
		}
		if err != nil {
			s.log.Warn("email outbox delivery failed", "message_id", m.ID, "error", err)
		}
	}
}
