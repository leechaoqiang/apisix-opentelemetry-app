# APISIX + OpenTelemetry 全链路可观测项目

基于 Apache APISIX 网关的微服务架构演示项目，集成 OpenTelemetry 实现全链路追踪和日志收集。

## 架构概览

```
用户请求
    ↓
APISIX 网关（路由、限流、鉴权、OTel 埋点、日志收集）
    ↓
    ├─> FastAPI (Python Agent) ───> Qwen LLM
    ├─> Gin (Go 服务)
    └─> SpringBoot (Java 服务) ───> Ollama 本地大模型

全链路可观测：
  Trace   → Jaeger (http://localhost:16686)
  Log     → Loki → Grafana (http://localhost:3000)
  Metric  → Prometheus → Grafana

日志收集架构：
  所有服务 → Loki Driver → Loki → Grafana
  APISIX   → http-logger 插件 → Loki
  Promtail → Docker Socket → Loki
```

## 前置要求

- Docker & Docker Compose v2+
- Go 1.22+
- Python 3.11+
- Java 17+
- Maven 3.8+
- Ollama (可选，用于调用本地大模型)

## 快速开始

### 1. 启动基础设施

```bash
# 在项目根目录执行
docker-compose up -d

# 查看服务状态
docker-compose ps

# 查看日志
docker-compose logs -f
```

启动后可访问：
| 服务 | 地址 | 说明 |
|------|------|------|
| APISIX Gateway | http://localhost:9080 | API 网关 |
| APISIX Admin | http://localhost:9180 | 管理接口 |
| APISIX Metrics | http://localhost:9091/apisix/prometheus/metrics | Prometheus 指标 |
| Jaeger UI | http://localhost:16686 | 链路追踪 |
| Prometheus | http://localhost:9090 | 指标查询 |
| Loki | http://localhost:3100/ready | 日志存储 |
| Grafana | http://localhost:3000 | 可视化面板 (admin/admin123) |

### 2. Grafana 配置

登录 Grafana (http://localhost:3000)：
- 用户名: `admin`
- 密码: `admin123`

数据源已自动配置：
- **Prometheus**: 指标数据
- **Loki**: 日志数据
- **Jaeger**: 链路追踪

#### 查看 Loki 日志

在 Grafana 中：
1. 进入 **Explore** 页面
2. 选择 **Loki** 数据源
3. 使用 LogQL 查询，例如：
   - `{service="apisix"}` - 查看 APISIX 日志
   - `{service="java-agent"}` - 查看 Java 服务日志
   - `{container="apisix"}` - 按容器查看

### 2. 启动 Python Agent

```bash
cd python-agent

# 创建虚拟环境（首次）
python -m venv venv
source venv/bin/activate  # Linux/macOS
# venv\Scripts\activate   # Windows

# 安装依赖（首次）
pip install fastapi uvicorn requests opentelemetry-api opentelemetry-sdk opentelemetry-exporter-otlp opentelemetry-instrumentation-fastapi opentelemetry-instrumentation-requests

# 启动服务
uvicorn main:app --host 0.0.0.0 --port 8000
```

### 3. 启动 Go Agent

```bash
cd go-agent

# 下载依赖（首次）
go mod tidy

# 开发模式运行
go run main.go

# 生产编译
go build -o go-app
./go-app
```

#### Go Agent 环境变量

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `OTEL_SERVICE_NAME` | `go-service` | 服务名称 |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `localhost:4317` | OpenTelemetry Collector 地址 |
| `PORT` | `8081` | 服务端口 |
| `JAVA_SERVICE_URL` | `http://localhost:8082` | Java 服务地址 |
| `OLLAMA_URL` | `http://localhost:11434` | Ollama 地址 |

#### Go Agent API 接口

| 接口 | 方法 | 说明 |
|------|------|------|
| `/health` | GET | 健康检查 |
| `/api/go/tool` | POST | 工具接口（用于测试链路追踪） |
| `/api/go/chat` | POST | 调用 Ollama 大模型 |
| `/api/go/call-java` | POST | 调用 Java 服务（演示跨服务调用） |
| `/config` | GET | 查看配置信息 |

### 4. 启动 Java Agent

```bash
cd java-agent

# 编译项目（首次）
mvn clean package -DskipTests

# 开发模式运行
mvn spring-boot:run

# 带 OTel 追踪运行（推荐）
OTEL_SERVICE_NAME=java-service \
OTEL_TRACES_EXPORTER=otlp \
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 \
mvn spring-boot:run

# 或者直接运行 jar
java -jar target/java-agent-1.0.0.jar
```

#### Ollama 配置

Java Agent 默认连接本地 Ollama 服务。启动前请确保：

```bash
# 安装 Ollama (macOS/Linux)
curl -fsSL https://ollama.com/install.sh | sh

# 启动 Ollama 并下载模型
ollama serve
ollama pull qwen3:4b

# 或使用其他模型
ollama pull llama3.2
```

可通过环境变量覆盖配置：

```bash
OLLAMA_BASE_URL=http://localhost:11434 \
OLLAMA_MODEL=qwen2.5:7b \
mvn spring-boot:run
```

## API 测试

### 通过 APISIX 网关访问（推荐）

```bash
# 调用 Python Agent
curl -X POST "http://localhost:9080/agent/chat?query=hello"

# 调用 Go Agent
curl -X POST http://localhost:9080/api/go/tool -d "{}"

# Go Agent 调用 Ollama 大模型
curl -X POST http://localhost:9080/api/go/chat \
  -H "Content-Type: application/json" \
  -d '{"query": "你好"}'

# Go Agent 调用 Java 服务 (演示跨服务链路追踪)
curl -X POST http://localhost:9080/api/go/call-java

# 调用 Java Agent (调用 Ollama 大模型)
curl -X POST http://localhost:9080/api/java/chat \
  -H "Content-Type: application/json" \
  -d '{"query": "你好，请介绍一下自己"}'

# Java Agent 工具接口
curl -X POST http://localhost:9080/api/java/tool -d "{}"
```

### 直接访问服务

```bash
# Python Agent 直连
curl -X POST "http://localhost:8000/agent/chat?query=hello"

# Go Agent 直连
curl http://localhost:8081/health
curl -X POST http://localhost:8081/api/go/tool -d "{}"
curl -X POST http://localhost:8081/api/go/chat \
  -H "Content-Type: application/json" \
  -d '{"query": "你好"}'
curl -X POST http://localhost:8081/api/go/call-java

# Java Agent 直连
curl -X POST http://localhost:8082/api/java/chat \
  -H "Content-Type: application/json" \
  -d '{"query": "帮我讲一个笑话"}'

curl -X POST http://localhost:8082/api/java/tool -d "{}"

# Java Agent 健康检查
curl http://localhost:8082/api/java/health

# 查看可用模型
curl http://localhost:8082/api/java/models
```

## APISIX 路由配置

| 路由规则 | 上游服务 |
|---------|---------|
| `/agent/*` | Python FastAPI (:8000) |
| `/api/go/*` | Go Gin (:8081) |
| `/api/java/*` | Java Service (:8082) |

### Admin API 操作

```bash
# 查看所有路由
curl -H "X-API-KEY: zROmjQmfUScormfShcCxRsbwApcdSDqC" \
  http://localhost:9180/apisix/admin/routes

# 添加新路由
curl -X PUT "http://localhost:9180/apisix/admin/routes/1" \
  -H "X-API-KEY: zROmjQmfUScormfShcCxRsbwApcdSDqC" \
  -H "Content-Type: application/json" \
  -d '{"uri":"/test/*", "upstream":{"nodes":{"host.docker.internal:9000":1}}}'

# 添加 /api/go/* 路由
curl http://localhost:9180/apisix/admin/routes/1 \
  -H 'X-API-KEY: zROmjQmfUScormfShcCxRsbwApcdSDqC' \
  -X PUT -d '{
    "uri": "/api/go/*",
    "upstream": {
      "type": "roundrobin",
      "nodes": {
        "host.docker.internal:8081": 1
      }
    }
  }'

# 添加 /api/java/* 路由
curl http://localhost:9180/apisix/admin/routes/2 \
  -H 'X-API-KEY: zROmjQmfUScormfShcCxRsbwApcdSDqC' \
  -X PUT -d '{
    "uri": "/api/java/*",
    "upstream": {
      "type": "roundrobin",
      "nodes": {
        "host.docker.internal:8082": 1
      }
    }
  }'

# 添加 /agent/* 路由
curl http://localhost:9180/apisix/admin/routes/3 \
  -H 'X-API-KEY: zROmjQmfUScormfShcCxRsbwApcdSDqC' \
  -X PUT -d '{
    "uri": "/agent/*",
    "upstream": {
      "type": "roundrobin",
      "nodes": {
        "host.docker.internal:8000": 1
      }
    }
  }'

```

## 运维命令

```bash
# 重启所有服务
docker-compose restart

# 停止所有服务
docker-compose down

# 查看日志
docker-compose logs -f apisix
docker-compose logs -f etcd
```

## 目录结构

```
.
├── docker-compose.yml          # 基础设施编排
├── apisix-config/              # APISIX 配置
│   └── config.yaml
├── prometheus/                 # Prometheus 配置
│   └── prometheus.yml
├── loki/                       # Loki 配置
│   ├── loki-config.yaml
│   └── promtail-config.yaml
├── grafana-data/               # Grafana 数据和配置
│   └── provisioning/
│       ├── datasources/
│       └── dashboards/
├── logs/                       # 日志目录
│   └── apisix/
├── etcd-data/                  # etcd 数据目录
└── apisix-apps/                # 后端服务
    ├── python-agent/           # FastAPI 服务
    │   └── main.py
    ├── go-agent/               # Gin 服务
    │   ├── main.go
    │   └── go.mod
    └── java-agent/             # SpringBoot 服务
        ├── pom.xml
        └── src/main/java/com/example/javaagent/
            ├── JavaAgentApplication.java
            ├── config/
            ├── controller/
            ├── dto/
            └── service/
```

## 配置说明

### Loki 日志收集

所有 Docker 服务的日志通过 Loki Driver 自动发送到 Loki：

```yaml
logging:
  driver: loki
  options:
    loki-url: "http://localhost:3100/loki/api/v1/push"
    labels: "service=apisix,environment=development"
```

### APISIX http-logger 插件

APISIX 通过 http-logger 插件将请求日志发送到 Loki：

```yaml
plugins:
  http-logger:
    enabled: true
    uri: http://loki:3100/loki/api/v1/push
    log_format:
      service: "apisix"
      route: "$uri"
      status: "$status"
```
