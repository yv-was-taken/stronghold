# Production Readiness Roadmap

This document tracks the remaining work needed to launch Stronghold in production.

## Critical (Must Fix Before Launch)

- [x] **Add test coverage** - Comprehensive test suite added
  - [x] Unit tests for `internal/handlers/` (auth, scan, account, health, pricing)
  - [x] Unit tests for `internal/db/` (accounts, sessions, deposits, usage)
  - [x] Unit tests for `internal/middleware/` (x402, ratelimit, requestid)
  - [x] Integration tests for API endpoints (`internal/server/server_integration_test.go`)
  - [x] Frontend component tests (Button, AuthProvider, Login page, utils)

- [x] **Fix JWT_SECRET dev fallback** - Now validates at startup
  - Added `ENV` environment variable (`development`/`production`/`test`)
  - Added `Config.Validate()` that fails if `JWT_SECRET` or `DB_PASSWORD` missing in production
  - Removed insecure fallback from `internal/handlers/auth.go`

## High Priority

- [x] **Add structured logging** - Migrated to `log/slog`
  - JSON output in production, text output in development
  - All files converted: main.go, server.go, worker.go, x402.go, proxy, stronghold client

- [x] **Add CI/CD test step** - Added `.github/workflows/test.yml`
  - [x] `go test -race ./...` with coverage reporting
  - [x] `golangci-lint` for code quality
  - [x] Frontend tests with Vitest (`bun run test:run`)
  - [ ] Consider consolidating duplicate workflow files

- [ ] **Complete dashboard** - `web/app/dashboard/main/`
  - [ ] Billing/usage history page (missing entirely)
  - [ ] Real Stripe checkout integration (currently placeholder at `internal/handlers/account.go:298`)
  - [ ] Error boundaries for React components
  - [ ] Better loading states

## Medium Priority

- [x] **Fix hardcoded pricing network** - Now uses `X402Middleware.GetNetwork()`

- [ ] **Add database migration tooling** - Only raw SQL file exists
  - Implement proper migration versioning (golang-migrate, Flyway, etc.)
  - Current: `internal/db/migrations/001_initial_schema.sql`

- [ ] **Expose API documentation** - OpenAPI comments exist but no endpoint
  - Add `/docs` endpoint with Swagger UI
  - Generate OpenAPI JSON spec

- [ ] **Add CSRF protection** - Dashboard forms lack CSRF tokens

- [ ] **Add Content-Security-Policy headers** - No CSP configured

- [ ] **Add database query timeouts** - Queries can hang indefinitely
  - Configure context timeouts for all DB operations

- [x] **Validate config at startup** - Added `Config.Validate()` method
  - Fails if `JWT_SECRET` or `DB_PASSWORD` missing in production
  - Called at startup in `main.go`

## Low Priority

- [ ] **Add distributed tracing** - OpenTelemetry integration
- [ ] **Add Prometheus metrics** - `/metrics` endpoint for monitoring
- [ ] **Document backup strategy** - No backup configuration exists
- [ ] **Add secret rotation plan** - No mechanism for rotating JWT_SECRET or DB credentials
- [ ] **Load testing** - Verify performance under expected traffic

## Deployment Checklist

Before going live:

- [x] All tests passing
- [x] `JWT_SECRET` configured (validated at startup in production)
- [x] `DB_PASSWORD` changed from default (validated at startup in production)
- [ ] Database migrations executed
- [ ] CORS origins configured for production domain
- [x] Rate limiting enabled
- [x] Structured logging verified
- [x] Health checks returning accurate status
- [ ] No secrets in git history
- [ ] SSL/TLS certificates configured
- [ ] Database backups configured
- [ ] Monitoring/alerting configured

## Architecture Notes

Current state:
- **Core scanning**: Functional with 4-layer detection (heuristics, ML, semantic, LLM)
- **Payment flow**: x402 integration with atomic settlement (reserve-commit pattern)
- **Database**: Well-structured schema with proper indexes and constraints
- **CLI/Proxy**: Transparent proxy implementation complete
- **Docker/Deployment**: Good configuration with health checks and resource limits

The foundation is solid. Primary gaps are in testing, security hardening, and observability.
