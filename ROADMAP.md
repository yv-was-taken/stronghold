# Production Readiness Roadmap

This document tracks the remaining work needed to launch Stronghold in production.

## Critical (Must Fix Before Launch)

- [ ] **Add test coverage** - No test files exist for Go backend or React frontend
  - [ ] Unit tests for `internal/handlers/` (auth, scan, account)
  - [ ] Unit tests for `internal/db/` (accounts, sessions, deposits, usage)
  - [ ] Unit tests for `internal/middleware/x402.go` (payment verification)
  - [ ] Integration tests for API endpoints
  - [ ] Frontend component tests

- [ ] **Fix JWT_SECRET dev fallback** - `internal/handlers/auth.go:31` falls back to insecure default
  - Change to `log.Fatal()` if `JWT_SECRET` env var is not set in production

## High Priority

- [ ] **Add structured logging** - Currently using basic `log` package
  - Switch to `log/slog` or `zap` with JSON output
  - Resolve TODO at `internal/handlers/auth.go:196`

- [ ] **Add CI/CD test step** - `.github/workflows/deploy.yml` and `fly-deploy.yml` deploy without testing
  - Add `go test ./...` step
  - Add `go vet` and `golangci-lint`
  - Add frontend `npm run build && npm run lint`
  - Consider consolidating duplicate workflow files

- [ ] **Complete dashboard** - `web/app/dashboard/main/`
  - [ ] Billing/usage history page (missing entirely)
  - [ ] Real Stripe checkout integration (currently placeholder at `internal/handlers/account.go:298`)
  - [ ] Error boundaries for React components
  - [ ] Better loading states

## Medium Priority

- [ ] **Fix hardcoded pricing network** - `internal/handlers/pricing.go:75`
  - TODO: "Get from config" - currently hardcodes "base-sepolia"

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

- [ ] **Validate config at startup** - `internal/config/config.go`
  - Add `Validate()` method to catch missing critical env vars
  - Fail fast if required config is missing

## Low Priority

- [ ] **Add distributed tracing** - OpenTelemetry integration
- [ ] **Add Prometheus metrics** - `/metrics` endpoint for monitoring
- [ ] **Document backup strategy** - No backup configuration exists
- [ ] **Add secret rotation plan** - No mechanism for rotating JWT_SECRET or DB credentials
- [ ] **Load testing** - Verify performance under expected traffic

## Deployment Checklist

Before going live:

- [ ] All tests passing
- [ ] `JWT_SECRET` configured (not using default)
- [ ] `DB_PASSWORD` changed from default
- [ ] Database migrations executed
- [ ] CORS origins configured for production domain
- [x] Rate limiting enabled
- [ ] Structured logging verified
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
