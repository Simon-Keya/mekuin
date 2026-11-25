package main

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type mpesaDispatch struct {
	AttemptID, Phone, AccountReference, Environment string
	AmountMinor                                     int64
}

// runMPesaDispatchWorker only sees committed outbox rows through narrow
// SECURITY DEFINER functions. It never retries an ambiguous STK submission.
func (s *server) runMPesaDispatchWorker(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		s.dispatchMPesaBatch(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *server) dispatchMPesaBatch(ctx context.Context) {
	// Periodic correlation is the final race backstop: under READ COMMITTED,
	// callback ingestion and dispatch completion can each miss the other's
	// uncommitted row in a narrow commit-order window. A later pass sees both.
	if _, err := s.db.Exec(ctx, `SELECT correlate_mpesa_unmatched_callbacks()`); err != nil {
		s.log.Error("M-Pesa callback correlation pass failed", "error", err)
	}
	for i := 0; i < 10; i++ {
		var m mpesaDispatch
		err := s.db.QueryRow(ctx, `SELECT attempt_id,phone_e164,amount_minor,account_reference,environment FROM claim_mpesa_dispatch()`).Scan(&m.AttemptID, &m.Phone, &m.AmountMinor, &m.AccountReference, &m.Environment)
		if err == pgx.ErrNoRows {
			return
		}
		if err != nil {
			s.log.Error("M-Pesa dispatch claim failed", "error", err)
			return
		}
		if m.AmountMinor <= 0 || m.AmountMinor%100 != 0 || m.AmountMinor/100 > 250000 || !validDarajaPhone(m.Phone) {
			_, err = s.db.Exec(ctx, `SELECT complete_mpesa_dispatch($1,false,'','',$2)`, m.AttemptID, "stored amount or phone failed dispatch validation")
			if err != nil {
				s.log.Error("M-Pesa dispatch failure update failed", "error", err)
			}
			continue
		}
		client, clientErr := newDarajaClientFromEnv()
		if clientErr == nil && ((m.Environment == "sandbox") != strings.Contains(client.baseURL, "sandbox.safaricom.co.ke")) {
			clientErr = errDarajaEnvironmentMismatch
		}
		var response darajaSTKResponse
		if clientErr == nil {
			callCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
			response, clientErr = client.initiateSTK(callCtx, m.Phone, m.AmountMinor/100, m.AccountReference, "MEKU POS", time.Now())
			cancel()
		}
		if clientErr != nil {
			_, err = s.db.Exec(ctx, `SELECT complete_mpesa_dispatch($1,false,'','',$2)`, m.AttemptID, clientErr.Error())
		} else {
			_, err = s.db.Exec(ctx, `SELECT complete_mpesa_dispatch($1,true,$2,$3,'')`, m.AttemptID, response.MerchantRequestID, response.CheckoutRequestID)
		}
		if err != nil {
			s.log.Error("M-Pesa dispatch completion failed", "attempt_id", m.AttemptID, "error", err)
		}
	}
}

var errDarajaEnvironmentMismatch = darajaEnvironmentError{}

type darajaEnvironmentError struct{}

func (darajaEnvironmentError) Error() string {
	return "Daraja environment does not match payment attempt"
}
