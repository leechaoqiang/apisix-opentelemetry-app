# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is an APISIX API Gateway project with OpenTelemetry integration for full-stack observability. It demonstrates distributed tracing across multiple backend services.

## Architecture

```
User Request → APISIX Gateway (port 9080) → Backend Services
                                                  ├── Python Agent (FastAPI, port 8000)
                                                  ├── Go Agent (Gin, port 8081)
                                                  └── Java Service (port 8082)

Observability Stack:
  - Traces → Jaeger (http://localhost:16686)
  - Logs → Loki (http://localhost:3100)
  - Metrics → Prometheus (http://localhost:9090)
  - Dashboard → Grafana (http://localhost:3000)
```

## Common Commands

### Infrastructure (Docker Compose)

```bash
# Start all services (etcd, APISIX, Jaeger, Prometheus, Loki, Grafana)
docker-compose up -d

# Stop all services
docker-compose down

# View logs
docker-compose logs -f apisix
```

### Python Agent (FastAPI)

```bash
cd apisix-apps/python-agent
uvicorn main:app --host 0.0.0.0 --port 8000
```

### Go Agent (Gin)

```bash
cd apisix-apps/go-agent

# Development
go run main.go

# With OpenTelemetry tracing (recommended)
OTEL_SERVICE_NAME=go-service \
OTEL_TRACES_EXPORTER=otlp \
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 \
go run main.go

# Production build
go build -o go-app
./go-app
```

### Test Requests

```bash
# Through APISIX gateway
curl -X POST "http://localhost:9080/agent/chat?query=hello"
curl -X POST http://localhost:9080/api/go/tool -d "{}"

# Direct to services (bypassing APISIX)
curl -X POST "http://localhost:8000/agent/chat?query=hello"
curl -X POST http://localhost:8081/api/go/tool -d "{}"
```

## APISIX Configuration

- **Gateway Port**: 9080
- **Admin API Port**: 9180
- **Admin Key**: `zROmjQmfUScormfShcCxRsbwApcdSDqC`
- **Config File**: `apisix-config/config.yaml`

### Routes

| Route Pattern | Upstream |
|---------------|----------|
| `/agent/*` | Python FastAPI (host.docker.internal:8000) |
| `/api/go/*` | Go Gin (host.docker.internal:8081) |
| `/api/java/*` | Java Service (host.docker.internal:8082) |

### Admin API Example

```bash
# List routes
curl -H "X-API-KEY: zROmjQmfUScormfShcCxRsbwApcdSDqC" http://localhost:9180/apisix/admin/routes

# Add a new route
curl -X PUT "http://localhost:9180/apisix/admin/routes/1" \
  -H "X-API-KEY: zROmjQmfUScormfShcCxRsbwApcdSDqC" \
  -d '{"uri":"/test/*", "upstream":{"nodes":{"host.docker.internal:9000":1}}}'
```

## OpenTelemetry Integration

All services export traces to Jaeger via OTLP protocol on `localhost:4317`. The Python and Go agents have auto-instrumentation configured. APISIX's OpenTelemetry plugin forwards trace context to downstream services.

## Service Dependencies

- APISIX requires etcd to be running first
- Backend services can run independently but need OTel collector for tracing
- Grafana depends on Prometheus, Loki, and Jaeger