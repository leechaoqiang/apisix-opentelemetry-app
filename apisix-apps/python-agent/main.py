from fastapi import FastAPI
import requests
import os
from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
from opentelemetry.instrumentation.requests import RequestsInstrumentor
from opentelemetry.sdk.resources import Resource

# 从环境变量读取配置
SERVICE_NAME = os.getenv("SERVICE_NAME", "python-agent")
JAEGER_ENDPOINT = os.getenv("JAEGER_ENDPOINT", "127.0.0.1:4317")
LOG_LEVEL = os.getenv("LOG_LEVEL", "INFO")

# 初始化 Trace - 关键：设置服务名称
resource = Resource.create({
    "service.name": SERVICE_NAME,
    "service.version": "1.0.0",
    "deployment.environment": os.getenv("ENVIRONMENT", "development"),
})

trace.set_tracer_provider(TracerProvider(resource=resource))

span_processor = BatchSpanProcessor(
    OTLPSpanExporter(
        endpoint=JAEGER_ENDPOINT,
        insecure=True  # 开发环境使用 HTTP，生产环境改为 False 并配置证书
    )
)
trace.get_tracer_provider().add_span_processor(span_processor)

# 获取 tracer 用于手动埋点
tracer = trace.get_tracer(__name__)

app = FastAPI(
    title="Python Agent",
    description="APISIX 链路追踪示例服务",
    version="1.0.0"
)

# 自动注入 FastAPI 和 requests
FastAPIInstrumentor.instrument_app(app)
RequestsInstrumentor().instrument()

@app.get("/health")
async def health_check():
    """健康检查接口"""
    return {"status": "healthy", "service": SERVICE_NAME}

@app.post("/agent/chat")
async def chat(query: str):
    """聊天接口 - 包含手动埋点示例"""
    # 手动创建 span（可选，自动埋点已包含大部分信息）
    with tracer.start_as_current_span("chat_operation") as span:
        span.set_attribute("query.length", len(query))
        span.set_attribute("custom.tag", "python-agent")
        
        resp = requests.post(
            "http://localhost:9080/api/go/tool",  # 使用 Docker 服务名
            json={"query": query},
            timeout=5
        )
        
        span.set_attribute("response.status", resp.status_code)
        
        return {"trace_from_agent": resp.json(), "service": SERVICE_NAME}

@app.get("/trace/info")
async def trace_info():
    """返回当前 Trace 配置信息"""
    return {
        "service_name": SERVICE_NAME,
        "jaeger_endpoint": JAEGER_ENDPOINT,
        "environment": os.getenv("ENVIRONMENT", "development"),
        "jaeger_ui": "http://localhost:16686"  # Jaeger UI 地址
    }