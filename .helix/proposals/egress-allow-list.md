---
slug: egress-allow-list
title: Allow egress to api.example.com for the hello-system
status: open
kit_version: 1.0.0
summary: Reference proposal demonstrating the proposal-back mechanism.
---

## What

hello-system would like to call `https://api.example.com/v1/echo` to fan out
its echo to an external service.

## Why it makes sense for Helix

This is a stand-in for any spoke that needs to reach an external HTTP API.
A generic `external-http-mcp` server that the Gateway can dispatch to would
let every system use the same pattern.

## Proposed shape

```
POST /invoke/external-http-mcp/<host>/<path>
{
  "method": "POST",
  "body":   { "...": "..." },
  "headers": { "...": "..." }
}
```

The MCP server validates the host against an allow-list maintained by the
Platform.Admin, applies egress-classification rules to the body, and proxies
the call. Audit log entry captures the host + path + classification.
