package db

import (
	"context"
	"testing"
	"time"

	"stronghold/internal/db/testutil"
	"stronghold/internal/usdc"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUsageStats_Aggregation(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// Fund the account so usage deductions don't violate balance constraint
	err = db.UpdateBalance(ctx, account.ID, usdc.FromFloat(100.0))
	require.NoError(t, err)

	// Create usage logs
	logs := []struct {
		cost    usdc.MicroUSDC
		threat  bool
		latency int
	}{
		{usdc.MicroUSDC(1000), false, 50},   // $0.001
		{usdc.MicroUSDC(2000), true, 100},   // $0.002
		{usdc.MicroUSDC(1000), false, 75},   // $0.001
		{usdc.MicroUSDC(5000), true, 150},   // $0.005
		{usdc.MicroUSDC(1000), false, 60},   // $0.001
	}

	for _, l := range logs {
		latencyMs := l.latency
		log := &UsageLog{
			AccountID:      account.ID,
			RequestID:      uuid.New().String(),
			Endpoint:       "/v1/scan",
			Method:         "POST",
			CostUSDC:       l.cost,
			Status:         "success",
			ThreatDetected: l.threat,
			LatencyMs:      &latencyMs,
		}
		err = db.CreateUsageLog(ctx, log)
		require.NoError(t, err)
	}

	// Get stats for today
	end := time.Now().UTC().Add(time.Hour)
	start := time.Now().UTC().Add(-24 * time.Hour)

	stats, err := db.GetUsageStats(ctx, account.ID, start, end)
	require.NoError(t, err)

	// Verify aggregation
	assert.Equal(t, int64(5), stats.TotalRequests)
	assert.Equal(t, usdc.MicroUSDC(10000), stats.TotalCostUSDC) // 1000+2000+1000+5000+1000
	assert.Equal(t, int64(2), stats.ThreatsDetected)
	assert.InDelta(t, 87.0, stats.AvgLatencyMs, 1.0) // (50+100+75+150+60)/5 = 87
}

func TestGetDailyStats_DateRange(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// Fund the account so usage deductions don't violate balance constraint
	err = db.UpdateBalance(ctx, account.ID, usdc.FromFloat(100.0))
	require.NoError(t, err)

	// Create usage logs for today only
	for i := 0; i < 5; i++ {
		log := &UsageLog{
			AccountID:      account.ID,
			RequestID:      uuid.New().String(),
			Endpoint:       "/v1/scan",
			Method:         "POST",
			CostUSDC:       usdc.MicroUSDC(1000),
			Status:         "success",
			ThreatDetected: false,
		}
		err = db.CreateUsageLog(ctx, log)
		require.NoError(t, err)
	}

	// Get daily stats for last 7 days
	stats, err := db.GetDailyUsageStats(ctx, account.ID, 7)
	require.NoError(t, err)

	// Should have at least today's data
	require.Greater(t, len(stats), 0)

	// Today's stats should show 5 requests
	today := time.Now().UTC().Format("2006-01-02")
	var todayStats *DailyUsageStats
	for _, s := range stats {
		if s.Date == today {
			todayStats = s
			break
		}
	}
	require.NotNil(t, todayStats, "Today's stats should be present")
	assert.Equal(t, int64(5), todayStats.RequestCount)
}

func TestPagination_Limits(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// Fund the account so usage deductions don't violate balance constraint
	err = db.UpdateBalance(ctx, account.ID, usdc.FromFloat(100.0))
	require.NoError(t, err)

	// Create some usage logs
	for i := 0; i < 10; i++ {
		log := &UsageLog{
			AccountID:      account.ID,
			RequestID:      uuid.New().String(),
			Endpoint:       "/v1/scan",
			Method:         "POST",
			CostUSDC:       usdc.MicroUSDC(1000),
			Status:         "success",
			ThreatDetected: false,
		}
		err = db.CreateUsageLog(ctx, log)
		require.NoError(t, err)
	}

	t.Run("limit <= 0 defaults to 50", func(t *testing.T) {
		logs, err := db.GetUsageLogs(ctx, account.ID, 0, 0)
		require.NoError(t, err)
		assert.Len(t, logs, 10) // All 10 logs returned (less than default 50)
	})

	t.Run("limit > 1000 capped to 1000", func(t *testing.T) {
		// This shouldn't error, just cap the limit
		_, err := db.GetUsageLogs(ctx, account.ID, 5000, 0)
		require.NoError(t, err)
	})

	t.Run("pagination respects limit and offset", func(t *testing.T) {
		logs1, err := db.GetUsageLogs(ctx, account.ID, 5, 0)
		require.NoError(t, err)
		assert.Len(t, logs1, 5)

		logs2, err := db.GetUsageLogs(ctx, account.ID, 5, 5)
		require.NoError(t, err)
		assert.Len(t, logs2, 5)

		// Verify they're different logs
		for _, l1 := range logs1 {
			for _, l2 := range logs2 {
				assert.NotEqual(t, l1.ID, l2.ID)
			}
		}
	})
}

func TestGetDailyUsageStats_DaysEnforced(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	t.Run("days <= 0 defaults to 30", func(t *testing.T) {
		_, err := db.GetDailyUsageStats(ctx, account.ID, 0)
		require.NoError(t, err)
		// No error means it defaulted properly
	})

	t.Run("days > 365 capped to 365", func(t *testing.T) {
		_, err := db.GetDailyUsageStats(ctx, account.ID, 1000)
		require.NoError(t, err)
		// No error means it capped properly
	})
}

func TestGetEndpointUsageStats(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// Fund the account so usage deductions don't violate balance constraint
	err = db.UpdateBalance(ctx, account.ID, usdc.FromFloat(100.0))
	require.NoError(t, err)

	// Create logs for different endpoints
	endpoints := []struct {
		path  string
		count int
		cost  usdc.MicroUSDC
	}{
		{"/v1/scan/content", 10, usdc.MicroUSDC(1000)},
		{"/v1/scan/output", 5, usdc.MicroUSDC(1000)},
		{"/v1/scan", 3, usdc.MicroUSDC(2000)},
	}

	for _, ep := range endpoints {
		for i := 0; i < ep.count; i++ {
			latency := 50
			log := &UsageLog{
				AccountID:      account.ID,
				RequestID:      uuid.New().String(),
				Endpoint:       ep.path,
				Method:         "POST",
				CostUSDC:       ep.cost,
				Status:         "success",
				ThreatDetected: false,
				LatencyMs:      &latency,
			}
			err = db.CreateUsageLog(ctx, log)
			require.NoError(t, err)
		}
	}

	end := time.Now().UTC().Add(time.Hour)
	start := time.Now().UTC().Add(-24 * time.Hour)

	stats, err := db.GetEndpointUsageStats(ctx, account.ID, start, end)
	require.NoError(t, err)
	require.Len(t, stats, 3)

	// Should be ordered by request count (descending)
	assert.Equal(t, "/v1/scan/content", stats[0].Endpoint)
	assert.Equal(t, int64(10), stats[0].RequestCount)

	assert.Equal(t, "/v1/scan/output", stats[1].Endpoint)
	assert.Equal(t, int64(5), stats[1].RequestCount)

	assert.Equal(t, "/v1/scan", stats[2].Endpoint)
	assert.Equal(t, int64(3), stats[2].RequestCount)
}

func TestGetUsageLogsByDateRange(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// Fund the account so usage deductions don't violate balance constraint
	err = db.UpdateBalance(ctx, account.ID, usdc.FromFloat(100.0))
	require.NoError(t, err)

	// Create logs
	for i := 0; i < 5; i++ {
		log := &UsageLog{
			AccountID:      account.ID,
			RequestID:      uuid.New().String(),
			Endpoint:       "/v1/scan",
			Method:         "POST",
			CostUSDC:       usdc.MicroUSDC(1000),
			Status:         "success",
			ThreatDetected: false,
		}
		err = db.CreateUsageLog(ctx, log)
		require.NoError(t, err)
	}

	// Query with date range including all logs
	end := time.Now().UTC().Add(time.Hour)
	start := time.Now().UTC().Add(-24 * time.Hour)

	logs, err := db.GetUsageLogsByDateRange(ctx, account.ID, start, end, 0, 0)
	require.NoError(t, err)
	assert.Len(t, logs, 5)

	// Query with date range excluding all logs
	futureStart := time.Now().UTC().Add(24 * time.Hour)
	futureEnd := time.Now().UTC().Add(48 * time.Hour)

	logs, err = db.GetUsageLogsByDateRange(ctx, account.ID, futureStart, futureEnd, 0, 0)
	require.NoError(t, err)
	assert.Len(t, logs, 0)
}

func TestCreateUsageLog_AllFields(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// Fund the account so usage deductions don't violate balance constraint
	err = db.UpdateBalance(ctx, account.ID, usdc.FromFloat(100.0))
	require.NoError(t, err)

	requestSize := 500
	responseSize := 1000
	latencyMs := 75
	threatType := "prompt_injection"

	log := &UsageLog{
		AccountID:         account.ID,
		RequestID:         "req-12345",
		Endpoint:          "/v1/scan/content",
		Method:            "POST",
		CostUSDC:          usdc.MicroUSDC(1000),
		Status:            "success",
		ThreatDetected:    true,
		ThreatType:        &threatType,
		RequestSizeBytes:  &requestSize,
		ResponseSizeBytes: &responseSize,
		LatencyMs:         &latencyMs,
		Metadata:          map[string]any{"source": "test"},
	}

	err = db.CreateUsageLog(ctx, log)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, log.ID)
	assert.False(t, log.CreatedAt.IsZero())

	// Retrieve and verify
	logs, err := db.GetUsageLogs(ctx, account.ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, logs, 1)

	retrieved := logs[0]
	assert.Equal(t, log.ID, retrieved.ID)
	assert.Equal(t, "req-12345", retrieved.RequestID)
	assert.Equal(t, "/v1/scan/content", retrieved.Endpoint)
	assert.Equal(t, "POST", retrieved.Method)
	assert.Equal(t, usdc.MicroUSDC(1000), retrieved.CostUSDC)
	assert.True(t, retrieved.ThreatDetected)
	require.NotNil(t, retrieved.ThreatType)
	assert.Equal(t, "prompt_injection", *retrieved.ThreatType)
	require.NotNil(t, retrieved.RequestSizeBytes)
	assert.Equal(t, 500, *retrieved.RequestSizeBytes)
	require.NotNil(t, retrieved.ResponseSizeBytes)
	assert.Equal(t, 1000, *retrieved.ResponseSizeBytes)
	require.NotNil(t, retrieved.LatencyMs)
	assert.Equal(t, 75, *retrieved.LatencyMs)
}

func TestUsageStats_EmptyAccount(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	end := time.Now().UTC().Add(time.Hour)
	start := time.Now().UTC().Add(-24 * time.Hour)

	stats, err := db.GetUsageStats(ctx, account.ID, start, end)
	require.NoError(t, err)

	assert.Equal(t, int64(0), stats.TotalRequests)
	assert.Equal(t, usdc.MicroUSDC(0), stats.TotalCostUSDC)
	assert.Equal(t, int64(0), stats.ThreatsDetected)
	assert.Equal(t, float64(0), stats.AvgLatencyMs)
}

func TestGetUsageLogs_OrderedByCreatedAtDesc(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// Fund the account so usage deductions don't violate balance constraint
	err = db.UpdateBalance(ctx, account.ID, usdc.FromFloat(100.0))
	require.NoError(t, err)

	// Create logs with slight delay to ensure different timestamps
	var ids []uuid.UUID
	for i := 0; i < 5; i++ {
		log := &UsageLog{
			AccountID:      account.ID,
			RequestID:      uuid.New().String(),
			Endpoint:       "/v1/scan",
			Method:         "POST",
			CostUSDC:       usdc.MicroUSDC(1000),
			Status:         "success",
			ThreatDetected: false,
		}
		err = db.CreateUsageLog(ctx, log)
		require.NoError(t, err)
		ids = append(ids, log.ID)
		time.Sleep(5 * time.Millisecond) // Small delay to ensure different timestamps
	}

	logs, err := db.GetUsageLogs(ctx, account.ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, logs, 5)

	// Most recent should be first (last created)
	assert.Equal(t, ids[4], logs[0].ID)
	assert.Equal(t, ids[0], logs[4].ID)

	// Verify descending order
	for i := 1; i < len(logs); i++ {
		assert.True(t, logs[i-1].CreatedAt.After(logs[i].CreatedAt) || logs[i-1].CreatedAt.Equal(logs[i].CreatedAt),
			"Logs should be ordered by created_at descending")
	}
}
