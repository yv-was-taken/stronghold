package db

import (
	"context"
	"fmt"
	"time"

	"stronghold/internal/usdc"

	"github.com/google/uuid"
)

// UsageLog represents a single API usage record
type UsageLog struct {
	ID                uuid.UUID      `json:"id"`
	AccountID         uuid.UUID      `json:"account_id"`
	RequestID         string         `json:"request_id"`
	Endpoint          string         `json:"endpoint"`
	Method            string         `json:"method"`
	CostUSDC          usdc.MicroUSDC `json:"cost_usdc"`
	Status            string         `json:"status"`
	ThreatDetected    bool           `json:"threat_detected"`
	ThreatType        *string        `json:"threat_type,omitempty"`
	RequestSizeBytes  *int           `json:"request_size_bytes,omitempty"`
	ResponseSizeBytes *int           `json:"response_size_bytes,omitempty"`
	LatencyMs         *int           `json:"latency_ms,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
}

// UsageStats represents aggregated usage statistics
type UsageStats struct {
	TotalRequests   int64          `json:"total_requests"`
	TotalCostUSDC   usdc.MicroUSDC `json:"total_cost_usdc"`
	ThreatsDetected int64          `json:"threats_detected"`
	AvgLatencyMs    float64        `json:"avg_latency_ms"`
}

// CreateUsageLog creates a new usage log entry
func (db *DB) CreateUsageLog(ctx context.Context, log *UsageLog) error {
	log.ID = uuid.New()
	log.CreatedAt = time.Now().UTC()

	_, err := db.pool.Exec(ctx, `
		INSERT INTO usage_logs (
			id, account_id, request_id, endpoint, method, cost_usdc, status,
			threat_detected, threat_type, request_size_bytes, response_size_bytes,
			latency_ms, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, log.ID, log.AccountID, log.RequestID, log.Endpoint, log.Method,
		log.CostUSDC, log.Status, log.ThreatDetected, log.ThreatType,
		log.RequestSizeBytes, log.ResponseSizeBytes, log.LatencyMs,
		log.Metadata, log.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create usage log: %w", err)
	}

	return nil
}

// GetUsageLogs retrieves usage logs for an account with pagination
func (db *DB) GetUsageLogs(ctx context.Context, accountID uuid.UUID, limit, offset int) ([]*UsageLog, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	rows, err := db.pool.Query(ctx, `
		SELECT id, account_id, request_id, endpoint, method, cost_usdc, status,
		       threat_detected, threat_type, request_size_bytes, response_size_bytes,
		       latency_ms, metadata, created_at
		FROM usage_logs
		WHERE account_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, accountID, limit, offset)

	if err != nil {
		return nil, fmt.Errorf("failed to get usage logs: %w", err)
	}
	defer rows.Close()

	var logs []*UsageLog
	for rows.Next() {
		log := &UsageLog{}
		err := rows.Scan(
			&log.ID, &log.AccountID, &log.RequestID, &log.Endpoint, &log.Method,
			&log.CostUSDC, &log.Status, &log.ThreatDetected, &log.ThreatType,
			&log.RequestSizeBytes, &log.ResponseSizeBytes, &log.LatencyMs,
			&log.Metadata, &log.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan usage log: %w", err)
		}
		logs = append(logs, log)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating usage logs: %w", err)
	}

	return logs, nil
}

// GetUsageLogsByDateRange retrieves usage logs within a date range with pagination.
// If limit is 0, it defaults to 1000. Offset defaults to 0.
func (db *DB) GetUsageLogsByDateRange(ctx context.Context, accountID uuid.UUID, start, end time.Time, limit, offset int) ([]*UsageLog, error) {
	if limit <= 0 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := db.pool.Query(ctx, `
		SELECT id, account_id, request_id, endpoint, method, cost_usdc, status,
		       threat_detected, threat_type, request_size_bytes, response_size_bytes,
		       latency_ms, metadata, created_at
		FROM usage_logs
		WHERE account_id = $1 AND created_at >= $2 AND created_at <= $3
		ORDER BY created_at DESC
		LIMIT $4 OFFSET $5
	`, accountID, start, end, limit, offset)

	if err != nil {
		return nil, fmt.Errorf("failed to get usage logs: %w", err)
	}
	defer rows.Close()

	var logs []*UsageLog
	for rows.Next() {
		log := &UsageLog{}
		err := rows.Scan(
			&log.ID, &log.AccountID, &log.RequestID, &log.Endpoint, &log.Method,
			&log.CostUSDC, &log.Status, &log.ThreatDetected, &log.ThreatType,
			&log.RequestSizeBytes, &log.ResponseSizeBytes, &log.LatencyMs,
			&log.Metadata, &log.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan usage log: %w", err)
		}
		logs = append(logs, log)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating usage logs: %w", err)
	}

	return logs, nil
}

// GetUsageStats retrieves aggregated usage statistics for an account
func (db *DB) GetUsageStats(ctx context.Context, accountID uuid.UUID, start, end time.Time) (*UsageStats, error) {
	stats := &UsageStats{}

	err := db.QueryRow(ctx, `
		SELECT
			COALESCE(COUNT(*), 0) as total_requests,
			COALESCE(SUM(cost_usdc), 0) as total_cost_usdc,
			COALESCE(SUM(CASE WHEN threat_detected THEN 1 ELSE 0 END), 0) as threats_detected,
			COALESCE(AVG(latency_ms), 0) as avg_latency_ms
		FROM usage_logs
		WHERE account_id = $1 AND created_at >= $2 AND created_at <= $3
	`, accountID, start, end).Scan(
		&stats.TotalRequests,
		&stats.TotalCostUSDC,
		&stats.ThreatsDetected,
		&stats.AvgLatencyMs,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get usage stats: %w", err)
	}

	return stats, nil
}

// GetDailyUsageStats retrieves daily usage statistics for an account
func (db *DB) GetDailyUsageStats(ctx context.Context, accountID uuid.UUID, days int) ([]*DailyUsageStats, error) {
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}

	startDate := time.Now().UTC().AddDate(0, 0, -days)

	rows, err := db.pool.Query(ctx, `
		SELECT
			DATE(created_at) as date,
			COUNT(*) as request_count,
			SUM(cost_usdc) as total_cost_usdc,
			SUM(CASE WHEN threat_detected THEN 1 ELSE 0 END) as threats_detected
		FROM usage_logs
		WHERE account_id = $1 AND created_at >= $2
		GROUP BY DATE(created_at)
		ORDER BY date DESC
	`, accountID, startDate)

	if err != nil {
		return nil, fmt.Errorf("failed to get daily usage stats: %w", err)
	}
	defer rows.Close()

	var stats []*DailyUsageStats
	for rows.Next() {
		stat := &DailyUsageStats{}
		var date time.Time
		err := rows.Scan(&date, &stat.RequestCount, &stat.TotalCostUSDC, &stat.ThreatsDetected)
		if err != nil {
			return nil, fmt.Errorf("failed to scan daily usage stats: %w", err)
		}
		stat.Date = date.Format("2006-01-02")
		stats = append(stats, stat)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating daily usage stats: %w", err)
	}

	return stats, nil
}

// DailyUsageStats represents usage statistics for a single day
type DailyUsageStats struct {
	Date            string         `json:"date"`
	RequestCount    int64          `json:"request_count"`
	TotalCostUSDC   usdc.MicroUSDC `json:"total_cost_usdc"`
	ThreatsDetected int64          `json:"threats_detected"`
}

// GetEndpointUsageStats retrieves usage statistics grouped by endpoint
func (db *DB) GetEndpointUsageStats(ctx context.Context, accountID uuid.UUID, start, end time.Time) ([]*EndpointUsageStats, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT
			endpoint,
			method,
			COUNT(*) as request_count,
			SUM(cost_usdc) as total_cost_usdc,
			AVG(latency_ms) as avg_latency_ms
		FROM usage_logs
		WHERE account_id = $1 AND created_at >= $2 AND created_at <= $3
		GROUP BY endpoint, method
		ORDER BY request_count DESC
	`, accountID, start, end)

	if err != nil {
		return nil, fmt.Errorf("failed to get endpoint usage stats: %w", err)
	}
	defer rows.Close()

	var stats []*EndpointUsageStats
	for rows.Next() {
		stat := &EndpointUsageStats{}
		err := rows.Scan(
			&stat.Endpoint, &stat.Method, &stat.RequestCount,
			&stat.TotalCostUSDC, &stat.AvgLatencyMs,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan endpoint usage stats: %w", err)
		}
		stats = append(stats, stat)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating endpoint usage stats: %w", err)
	}

	return stats, nil
}

// EndpointUsageStats represents usage statistics for a single endpoint
type EndpointUsageStats struct {
	Endpoint      string         `json:"endpoint"`
	Method        string         `json:"method"`
	RequestCount  int64          `json:"request_count"`
	TotalCostUSDC usdc.MicroUSDC `json:"total_cost_usdc"`
	AvgLatencyMs  float64        `json:"avg_latency_ms"`
}
