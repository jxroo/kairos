# Atlas Data Pipeline — Project Roadmap

## Overview

Atlas is a high-throughput data ingestion and processing platform designed to handle
real-time event streams from 40+ microservices. The system processes an average of
25,000 events per second and serves as the backbone for analytics, billing, and
audit logging across the organization.

## Technical Stack

- **Runtime:** Go 1.26 (ingestion workers), Python 3.13 (transformation layer)
- **Messaging:** Apache Kafka 3.9 (6-node cluster, 3 AZs)
- **Storage:** PostgreSQL 16 (metadata), ClickHouse (analytics), S3 (cold archive)
- **Orchestration:** Kubernetes 1.32 on AWS EKS
- **Monitoring:** Prometheus + Grafana, OpenTelemetry for distributed tracing

## Q1 Milestones (January — March 2026)

| Milestone | Owner | Target | Status |
|-----------|-------|--------|--------|
| Kafka consumer refactor | Elena | Feb 14 | Done |
| PostgreSQL 14 -> 16 migration | Marcus | Mar 6 | In Progress |
| Notification Service v2 API | Sofia | Mar 21 | In Progress |
| Integration test suite (>80% cov) | Sofia | Mar 25 | Not Started |
| Atlas v2.0 production release | Team | Mar 28 | Planned |

## Q2 Milestones (April — June 2026)

| Milestone | Owner | Target | Status |
|-----------|-------|--------|--------|
| ClickHouse cluster expansion | Marcus | Apr 15 | Planned |
| Real-time anomaly detection | Tomek | May 10 | Planned |
| Self-serve dashboard for analysts | Elena | May 30 | Planned |
| Event schema registry (Avro) | Sofia | Jun 15 | Planned |
| Performance audit & cost review | Team | Jun 30 | Planned |

## Team

- **Elena Vasquez** — Senior Backend Engineer, Kafka and ingestion layer
- **Marcus Brandt** — Infrastructure & Database Lead
- **Sofia Kowalska** — API Design & Integration Testing
- **Tomek Zielinski** — New hire, distributed systems (joined March 2026)
- **Project Lead** — reports to VP of Engineering

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| PostgreSQL migration causes downtime | Medium | High | Dry-run on staging, rollback scripts ready |
| Kafka consumer lag during peak hours | Low | Medium | Auto-scaling policies, backpressure handling |
| Security vulnerability in legacy importer | High | Critical | Fix before v2.0 release (SEC-142) |
| New team member ramp-up delays Q2 | Low | Low | Onboarding buddy system, documentation sprint |
| ClickHouse storage costs exceed budget | Medium | Medium | Implement TTL policies, cold-tier archival |
