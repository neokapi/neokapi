# Release Notes

## Version 3.2.0

We're excited to announce the latest release of Acme Cloud Platform with several new features and improvements.

### New Features

**Auto-scaling Groups** — You can now configure automatic scaling rules based on CPU usage, memory, or custom metrics. Set minimum and maximum instance counts and let the platform handle the rest.

**Regional Failover** — Projects can now be deployed across multiple regions with automatic failover. If the primary region becomes unavailable, traffic is seamlessly redirected to the backup region.

**Team Audit Logs** — All team actions are now logged with detailed audit trails. View who made changes, when they were made, and what was affected.

### Improvements

- Deployment times reduced by 40% through optimized container builds
- Dashboard now loads 3x faster with lazy-loading components
- Improved error messages for configuration validation

### Bug Fixes

- Fixed an issue where environment variables were not properly escaped in Docker builds
- Resolved a race condition in the WebSocket connection handler
- Fixed timezone display in the activity feed for users outside UTC

### Breaking Changes

The `v2/deployments` API endpoint has been deprecated. Please migrate to `v3/deployments` before March 2026. See our [migration guide](/docs/migration) for details.

### Known Issues

The new auto-scaling feature currently does not support custom metrics from third-party monitoring providers. Support for Datadog and Prometheus metrics is planned for version 3.3.
