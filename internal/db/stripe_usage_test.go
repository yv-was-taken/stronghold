package db

import (
	"context"
	"testing"

	"stronghold/internal/db/testutil"
	"stronghold/internal/usdc"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateStripeUsageRecord(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateB2BAccount(ctx, "user_01USAGE", "stripe-usage@example.com", "Usage Co")
	require.NoError(t, err)

	meterEventID := "evt_meter_123"

	record := &StripeUsageRecord{
		AccountID:          account.ID,
		StripeMeterEventID: &meterEventID,
		Endpoint:           "/v1/scan/content",
		AmountUSDC:         usdc.FromFloat(0.001),
	}

	created, err := db.CreateStripeUsageRecord(ctx, record)
	require.NoError(t, err)
	require.NotNil(t, created)

	// Verify fields
	assert.NotEqual(t, uuid.Nil, created.ID)
	assert.Equal(t, account.ID, created.AccountID)
	assert.Nil(t, created.UsageLogID) // no usage log linked in this test
	require.NotNil(t, created.StripeMeterEventID)
	assert.Equal(t, meterEventID, *created.StripeMeterEventID)
	assert.Equal(t, "/v1/scan/content", created.Endpoint)
	assert.Equal(t, usdc.FromFloat(0.001), created.AmountUSDC)
	assert.NotZero(t, created.CreatedAt)
}

func TestGetStripeUsageRecords(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateB2BAccount(ctx, "user_01RECORDS", "stripe-records@example.com", "Records Co")
	require.NoError(t, err)

	// Create multiple records
	endpoints := []string{"/v1/scan/content", "/v1/scan/output", "/v1/scan/content"}
	for _, ep := range endpoints {
		record := &StripeUsageRecord{
			AccountID:  account.ID,
			Endpoint:   ep,
			AmountUSDC: usdc.FromFloat(0.001),
		}
		_, err := db.CreateStripeUsageRecord(ctx, record)
		require.NoError(t, err)
	}

	// Retrieve all records
	records, err := db.GetStripeUsageRecords(ctx, account.ID, 50, 0)
	require.NoError(t, err)
	assert.Len(t, records, 3)

	// Verify records are ordered by created_at DESC
	for i := 0; i < len(records)-1; i++ {
		assert.True(t, records[i].CreatedAt.After(records[i+1].CreatedAt) || records[i].CreatedAt.Equal(records[i+1].CreatedAt),
			"records should be ordered by created_at DESC")
	}

	// Test pagination: limit 2, offset 0
	records, err = db.GetStripeUsageRecords(ctx, account.ID, 2, 0)
	require.NoError(t, err)
	assert.Len(t, records, 2)

	// Test pagination: limit 2, offset 2
	records, err = db.GetStripeUsageRecords(ctx, account.ID, 2, 2)
	require.NoError(t, err)
	assert.Len(t, records, 1)
}

func TestGetStripeUsageRecords_Empty(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateB2BAccount(ctx, "user_01EMPTY", "stripe-empty@example.com", "Empty Co")
	require.NoError(t, err)

	// Retrieve records for account with no usage
	records, err := db.GetStripeUsageRecords(ctx, account.ID, 50, 0)
	require.NoError(t, err)
	assert.Empty(t, records)
}
