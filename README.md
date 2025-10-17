# acserver-exporter

A Prometheus exporter for Assetto Corsa dedicated servers that monitors server metrics, player activity, and game events.

## Quick Start

**1. Edit `docker-compose.yml` and update the environment variables:**


| Variable | Description | Default |
|----------|-------------|---------|
| `AC_SERVER_HOST` | Assetto Corsa server IP/hostname | `127.0.0.1` |
| `AC_SERVER_UDP_PORT` | AC server UDP plugin port | `9600` |
| `AC_SERVER_HTTP_PORT` | AC server HTTP API port | `8081` |
| `METRICS_PORT` | Exporter metrics endpoint port | `9090` |


**2. Start the stack:**

```
docker compose up -d
```

- **Exporter Metrics**: http://localhost:9090/metrics
- **Exporter Health**: http://localhost:9090/health


**3. Update your `prometheus.yml` configuration**:

```
scrape_configs:
  - job_name: 'assetto-corsa'
    static_configs:
      - targets: ['acserver-exporter:9090']
```