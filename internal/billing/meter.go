package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"stronghold/internal/config"
	"stronghold/internal/db"
	"stronghold/internal/usdc"

	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/billing/meterevent"
)

// ErrMeteringNotConfigured is returned when Stripe metering config is missing.
var ErrMeteringNotConfigured = errors.New("stripe metering not configured")

// MeterReporter reports B2B API usage to Stripe's Meter API
type MeterReporter struct {
	db           *db.DB
	stripeConfig *config.StripeConfig
}

// NewMeterReporter creates a new meter reporter
func NewMeterReporter(database *db.DB, stripeConfig *config.StripeConfig) *MeterReporter {
	return &MeterReporter{
		db:           database,
		stripeConfig: stripeConfig,
	}
}

// IsConfigured returns whether Stripe metering is fully configured and ready
// to accept usage reports. Callers should check this before running billable
// work to avoid executing requests that cannot be billed.
func (m *MeterReporter) IsConfigured() bool {
	return m.stripeConfig.SecretKey != "" && m.stripeConfig.MeterEventName != ""
}

// ReportUsage reports a metered API usage event to Stripe and records it locally.
// The caller supplies a unique eventID (typically uuid.New()) for each billable
// event. Every call creates a distinct Stripe meter event — no deduplication is
// intended because the scan handler runs on every request, including retries.
func (m *MeterReporter) ReportUsage(ctx context.Context, accountID uuid.UUID, stripeCustomerID, endpoint string, amountMicroUSDC usdc.MicroUSDC, eventID string) error {
	if m.stripeConfig.SecretKey == "" || m.stripeConfig.MeterEventName == "" {
		return ErrMeteringNotConfigured
	}

	// Report raw microUSDC as the meter value to preserve sub-cent pricing
	// precision. The Stripe meter price must be configured to interpret
	// microUSDC units (1,000,000 = $1.00). Converting to cents first would
	// truncate all sub-cent prices (e.g. $0.001) to the same value.
	params := &stripe.BillingMeterEventParams{
		EventName:  stripe.String(m.stripeConfig.MeterEventName),
		Identifier: stripe.String(eventID),
		Payload: map[string]string{
			"stripe_customer_id": stripeCustomerID,
			"value":              fmt.Sprintf("%d", amountMicroUSDC),
		},
		Timestamp: stripe.Int64(time.Now().Unix()),
	}

	event, err := meterevent.New(params)

	var meterEventID *string
	var stripeErr error
	if err != nil {
		stripeErr = fmt.Errorf("stripe meter event failed: %w", err)
		slog.Error("failed to report usage to Stripe meter",
			"account_id", accountID,
			"endpoint", endpoint,
			"error", err,
		)
	} else if event != nil {
		meterEventID = &event.Identifier
	}

	// Record locally regardless of Stripe API result (for audit/reconciliation)
	record := &db.StripeUsageRecord{
		AccountID:          accountID,
		StripeMeterEventID: meterEventID,
		Endpoint:           endpoint,
		AmountUSDC:         amountMicroUSDC,
	}

	if _, err := m.db.CreateStripeUsageRecord(ctx, record); err != nil {
		slog.Error("failed to record stripe usage locally",
			"account_id", accountID,
			"endpoint", endpoint,
			"error", err,
		)
		// Only fail the request if Stripe also rejected the event.
		// When Stripe accepted it, failing here would cause a client retry
		// that submits a duplicate meter event (double charge).
		if stripeErr != nil {
			return fmt.Errorf("failed to record usage: %w", err)
		}
	}

	return stripeErr
}
