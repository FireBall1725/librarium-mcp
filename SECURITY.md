# Security policy

## Reporting a vulnerability

**Please report security issues privately**, not via public issues.

Use GitHub's private vulnerability reporting on this repo:

→ https://github.com/fireball1725/librarium-mcp/security/advisories/new

That keeps the report visible only to maintainers until a fix is ready, and gives us a paper trail to coordinate disclosure on.

## What's in scope

Anything that lets an attacker:

- Bypass the inbound bearer-token check on the MCP HTTP transport
- Use the MCP server to perform actions on the upstream Librarium API beyond what the configured PAT scope permits
- Leak the upstream `LIBRARIUM_ACCESS_TOKEN` to clients or logs
- Execute code on the MCP host through tool inputs
- Smuggle data through ISBN lookups or book searches that proxy through the API

For server-side issues that the upstream API is responsible for, file on [librarium-api](https://github.com/fireball1725/librarium-api/security/advisories/new) instead.

## Out of scope

- Issues that require admin access on the MCP host machine to exploit
- Findings from automated scanners that aren't reproducible against a real deployment
- DoS via volumetric traffic to a self-hosted instance

## Response

This is a small, self-hosted project run by a single maintainer. Best-effort response targets:

- **Acknowledgement**: within 1 week
- **Initial triage**: within 2 weeks
- **Fix or mitigation plan**: within 4 weeks for high-severity issues

We'll credit you in the release notes when the fix ships, unless you'd prefer to stay anonymous.
