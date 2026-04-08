# Sprint 14 Retrospective

**Date:** March 14, 2026, 10:00 AM
**Facilitator:** Elena Vasquez
**Attendees:** Elena Vasquez, Marcus Brandt, Sofia Kowalska, Tomek Zielinski
**Duration:** 45 minutes

---

## What Went Well

- Kafka consumer refactor landed on time with zero production incidents during rollout.
  Throughput improved by 2.5x compared to the previous implementation.
- Cross-team collaboration with the DevOps squad was smooth. The shared Slack channel
  for the PostgreSQL migration has been very effective.
- Sofia's new API contract testing approach caught two breaking changes before they
  hit staging. The team agreed to adopt this as a standard practice.
- Tomek's onboarding has been faster than expected. He already submitted his first
  PR (logging improvements in the event router).

## What Needs Improvement

- Code review turnaround is still too slow. Average time from PR open to first review
  is 26 hours — target is under 8 hours.
- The staging environment was down for 6 hours on Wednesday due to a misconfigured
  Terraform change. We need a staging health check in CI.
- Sprint planning estimates were off by 30% for the notification pipeline work.
  We underestimated the complexity of webhook retry logic.
- Documentation for the ingestion API is outdated. Three external teams reported
  confusion about the v1 vs v2 endpoint differences.

## Action Items

| Action | Owner | Due |
|--------|-------|-----|
| Set up PR review SLA alerts in GitHub | Elena | Mar 18 |
| Add staging health check to CI pipeline | Marcus | Mar 19 |
| Write webhook retry logic design doc | Sofia | Mar 17 |
| Update ingestion API docs (v1 vs v2) | Tomek | Mar 21 |
| Schedule security fix pairing session (SEC-142) | Elena + Marcus | Mar 16 |

## Next Sprint Priorities (Sprint 15)

1. **Critical:** Fix SQL injection in legacy batch importer (SEC-142)
2. **High:** Complete PostgreSQL 16 migration dry-run on staging
3. **High:** Notification Service v2 — webhook delivery with retry
4. **Medium:** Integration test coverage target 80%
5. **Low:** Explore ClickHouse materialized views for dashboard queries
