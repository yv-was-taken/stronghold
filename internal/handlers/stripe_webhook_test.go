package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"stronghold/internal/config"
	"stronghold/internal/db"
	"stronghold/internal/db/testutil"
	"stronghold/internal/usdc"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testWebhookSecret = "whsec_test_secret_key"

func setupStripeWebhookTest(t *testing.T) (*fiber.App, *StripeWebhookHandler, *testutil.TestDB, *db.DB) {
	testDB := testutil.NewTestDB(t)

	cfg := &db.Config{
		Host:     testDB.Host,
		Port:     testDB.Port,
		User:     testDB.User,
		Password: testDB.Password,
		Name:     testDB.Database,
		SSLMode:  "disable",
	}

	database, err := db.New(cfg)
	require.NoError(t, err)

	stripeConfig := &config.StripeConfig{
		SecretKey:      "sk_test_xxx",
		WebhookSecret:  testWebhookSecret,
		PublishableKey: "pk_test_xxx",
	}

	handler := NewStripeWebhookHandler(database, stripeConfig)

	app := fiber.New()
	app.Post("/webhooks/stripe", handler.HandleWebhook)

	return app, handler, testDB, database
}

// generateStripeSignature creates a valid Stripe webhook signature
func generateStripeSignature(payload []byte, secret string, timestamp int64) string {
	signedPayload := fmt.Sprintf("%d.%s", timestamp, string(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signedPayload))
	signature := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("t=%d,v1=%s", timestamp, signature)
}

func TestStripeWebhook_MissingSignature(t *testing.T) {
	app, _, testDB, database := setupStripeWebhookTest(t)
	defer testDB.Close(t)
	defer database.Close()

	payload := []byte(`{"type":"crypto.onramp_session.updated"}`)

	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	// No Stripe-Signature header

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	assert.Contains(t, body["error"], "Missing Stripe-Signature")
}

func TestStripeWebhook_InvalidSignature(t *testing.T) {
	app, _, testDB, database := setupStripeWebhookTest(t)
	defer testDB.Close(t)
	defer database.Close()

	payload := []byte(`{"type":"crypto.onramp_session.updated"}`)

	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Stripe-Signature", "t=1234567890,v1=invalid_signature")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	assert.Contains(t, body["error"], "Invalid signature")
}

func TestStripeWebhook_UnhandledEventType(t *testing.T) {
	app, _, testDB, database := setupStripeWebhookTest(t)
	defer testDB.Close(t)
	defer database.Close()

	timestamp := time.Now().Unix()
	payload := []byte(fmt.Sprintf(`{"id":"evt_test","type":"payment_intent.created","created":%d,"data":{"object":{}}}`, timestamp))
	signature := generateStripeSignature(payload, testWebhookSecret, timestamp)

	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Stripe-Signature", signature)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return 200 for unhandled events to prevent retries
	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	assert.True(t, body["received"].(bool))
}

func TestStripeWebhook_FulfillmentComplete(t *testing.T) {
	app, _, testDB, database := setupStripeWebhookTest(t)
	defer testDB.Close(t)
	defer database.Close()

	// Create an account
	account, err := database.CreateAccount(t.Context(), nil, nil)
	require.NoError(t, err)

	// Create a pending deposit
	deposit := &db.Deposit{
		AccountID:     account.ID,
		Provider:      db.DepositProviderStripe,
		AmountUSDC:    usdc.FromFloat(50.00),
		FeeUSDC:       usdc.FromFloat(1.75),
		NetAmountUSDC: usdc.FromFloat(48.25),
	}
	err = database.CreateDeposit(t.Context(), deposit)
	require.NoError(t, err)

	// Create webhook event payload
	timestamp := time.Now().Unix()
	payload := []byte(fmt.Sprintf(`{
		"id": "evt_test_fulfillment",
		"type": "crypto.onramp_session.updated",
		"created": %d,
		"data": {
			"object": {
				"id": "cos_test_session",
				"status": "fulfillment_complete",
				"metadata": {
					"deposit_id": "%s"
				}
			}
		}
	}`, timestamp, deposit.ID.String()))
	signature := generateStripeSignature(payload, testWebhookSecret, timestamp)

	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Stripe-Signature", signature)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	assert.True(t, body["received"].(bool))
	assert.Equal(t, "completed", body["status"])

	// Verify deposit is completed
	updatedDeposit, err := database.GetDepositByID(t.Context(), deposit.ID)
	require.NoError(t, err)
	assert.Equal(t, db.DepositStatusCompleted, updatedDeposit.Status)
	assert.NotNil(t, updatedDeposit.CompletedAt)

	// Verify account balance was credited
	updatedAccount, err := database.GetAccountByID(t.Context(), account.ID)
	require.NoError(t, err)
	assert.Equal(t, usdc.FromFloat(48.25), updatedAccount.BalanceUSDC)
}

func TestStripeWebhook_Idempotency(t *testing.T) {
	app, _, testDB, database := setupStripeWebhookTest(t)
	defer testDB.Close(t)
	defer database.Close()

	// Create an account
	account, err := database.CreateAccount(t.Context(), nil, nil)
	require.NoError(t, err)

	// Create a pending deposit
	deposit := &db.Deposit{
		AccountID:     account.ID,
		Provider:      db.DepositProviderStripe,
		AmountUSDC:    usdc.FromFloat(100.00),
		FeeUSDC:       usdc.FromFloat(3.20),
		NetAmountUSDC: usdc.FromFloat(96.80),
	}
	err = database.CreateDeposit(t.Context(), deposit)
	require.NoError(t, err)

	// Complete the deposit
	err = database.CompleteDeposit(t.Context(), deposit.ID)
	require.NoError(t, err)

	// Get account balance after first completion
	account1, err := database.GetAccountByID(t.Context(), account.ID)
	require.NoError(t, err)
	initialBalance := account1.BalanceUSDC

	// Send webhook for already-completed deposit
	timestamp := time.Now().Unix()
	payload := []byte(fmt.Sprintf(`{
		"id": "evt_test_duplicate",
		"type": "crypto.onramp_session.updated",
		"created": %d,
		"data": {
			"object": {
				"id": "cos_test_session",
				"status": "fulfillment_complete",
				"metadata": {
					"deposit_id": "%s"
				}
			}
		}
	}`, timestamp, deposit.ID.String()))
	signature := generateStripeSignature(payload, testWebhookSecret, timestamp)

	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Stripe-Signature", signature)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return 200 (idempotent)
	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	assert.True(t, body["received"].(bool))
	assert.Equal(t, "already_completed", body["status"])

	// Balance should not have changed (no double-credit)
	account2, err := database.GetAccountByID(t.Context(), account.ID)
	require.NoError(t, err)
	assert.Equal(t, initialBalance, account2.BalanceUSDC)
}

func TestStripeWebhook_Rejected(t *testing.T) {
	app, _, testDB, database := setupStripeWebhookTest(t)
	defer testDB.Close(t)
	defer database.Close()

	// Create an account
	account, err := database.CreateAccount(t.Context(), nil, nil)
	require.NoError(t, err)

	// Create a pending deposit
	deposit := &db.Deposit{
		AccountID:     account.ID,
		Provider:      db.DepositProviderStripe,
		AmountUSDC:    usdc.FromFloat(25.00),
		FeeUSDC:       usdc.FromFloat(1.03),
		NetAmountUSDC: usdc.FromFloat(23.97),
	}
	err = database.CreateDeposit(t.Context(), deposit)
	require.NoError(t, err)

	// Send rejected webhook
	timestamp := time.Now().Unix()
	payload := []byte(fmt.Sprintf(`{
		"id": "evt_test_rejected",
		"type": "crypto.onramp_session.updated",
		"created": %d,
		"data": {
			"object": {
				"id": "cos_test_session",
				"status": "rejected",
				"metadata": {
					"deposit_id": "%s"
				}
			}
		}
	}`, timestamp, deposit.ID.String()))
	signature := generateStripeSignature(payload, testWebhookSecret, timestamp)

	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Stripe-Signature", signature)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	assert.True(t, body["received"].(bool))
	assert.Equal(t, "failed", body["status"])

	// Verify deposit is marked as failed
	updatedDeposit, err := database.GetDepositByID(t.Context(), deposit.ID)
	require.NoError(t, err)
	assert.Equal(t, db.DepositStatusFailed, updatedDeposit.Status)
}

func TestStripeWebhook_IntermediateStatus(t *testing.T) {
	app, _, testDB, database := setupStripeWebhookTest(t)
	defer testDB.Close(t)
	defer database.Close()

	// Create an account
	account, err := database.CreateAccount(t.Context(), nil, nil)
	require.NoError(t, err)

	// Create a pending deposit
	deposit := &db.Deposit{
		AccountID:     account.ID,
		Provider:      db.DepositProviderStripe,
		AmountUSDC:    usdc.FromFloat(75.00),
		FeeUSDC:       usdc.FromFloat(2.48),
		NetAmountUSDC: usdc.FromFloat(72.52),
	}
	err = database.CreateDeposit(t.Context(), deposit)
	require.NoError(t, err)

	// Test intermediate statuses that should be ignored
	intermediateStatuses := []string{
		"requires_payment",
		"fulfillment_processing",
		"initialized",
	}

	for _, status := range intermediateStatuses {
		t.Run(status, func(t *testing.T) {
			timestamp := time.Now().Unix()
			payload := []byte(fmt.Sprintf(`{
				"id": "evt_test_%s",
				"type": "crypto.onramp_session.updated",
				"created": %d,
				"data": {
					"object": {
						"id": "cos_test_session",
						"status": "%s",
						"metadata": {
							"deposit_id": "%s"
						}
					}
				}
			}`, status, timestamp, status, deposit.ID.String()))

			signature := generateStripeSignature(payload, testWebhookSecret, timestamp)

			req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Stripe-Signature", signature)

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, 200, resp.StatusCode)

			var body map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&body)
			assert.True(t, body["received"].(bool))
			assert.Equal(t, "ignored", body["status"])

			// Deposit should still be pending
			updatedDeposit, err := database.GetDepositByID(t.Context(), deposit.ID)
			require.NoError(t, err)
			assert.Equal(t, db.DepositStatusPending, updatedDeposit.Status)
		})
	}
}

func TestStripeWebhook_MissingDepositID(t *testing.T) {
	app, _, testDB, database := setupStripeWebhookTest(t)
	defer testDB.Close(t)
	defer database.Close()

	// Webhook without deposit_id in metadata
	timestamp := time.Now().Unix()
	payload := []byte(fmt.Sprintf(`{
		"id": "evt_test_no_deposit_id",
		"type": "crypto.onramp_session.updated",
		"created": %d,
		"data": {
			"object": {
				"id": "cos_test_session",
				"status": "fulfillment_complete",
				"metadata": {}
			}
		}
	}`, timestamp))
	signature := generateStripeSignature(payload, testWebhookSecret, timestamp)

	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Stripe-Signature", signature)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return 200 to prevent retries (session wasn't created by us)
	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	assert.True(t, body["received"].(bool))
	assert.Contains(t, body["warning"], "missing deposit_id")
}

func TestStripeWebhook_InvalidDepositID(t *testing.T) {
	app, _, testDB, database := setupStripeWebhookTest(t)
	defer testDB.Close(t)
	defer database.Close()

	// Webhook with invalid deposit_id format
	timestamp := time.Now().Unix()
	payload := []byte(fmt.Sprintf(`{
		"id": "evt_test_invalid_id",
		"type": "crypto.onramp_session.updated",
		"created": %d,
		"data": {
			"object": {
				"id": "cos_test_session",
				"status": "fulfillment_complete",
				"metadata": {
					"deposit_id": "not-a-uuid"
				}
			}
		}
	}`, timestamp))
	signature := generateStripeSignature(payload, testWebhookSecret, timestamp)

	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Stripe-Signature", signature)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	assert.Contains(t, body["error"], "Invalid deposit_id")
}
