package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var tracer trace.Tracer

// Config 应用配置
type Config struct {
	ServiceName    string
	OtelEndpoint   string
	Port           string
	JavaServiceURL string
	OllamaURL      string
}

func loadConfig() *Config {
	return &Config{
		ServiceName:    getEnv("OTEL_SERVICE_NAME", "go-service"),
		OtelEndpoint:   getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		Port:           getEnv("PORT", "8081"),
		JavaServiceURL: getEnv("JAVA_SERVICE_URL", "http://localhost:9080"),
		OllamaURL:      getEnv("OLLAMA_URL", "http://localhost:11434"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// 初始化 OpenTelemetry
func initOTel(serviceName, endpoint string) (*sdktrace.TracerProvider, error) {
	// 创建资源 (不使用 SchemaURL 避免冲突)
	res := resource.NewWithAttributes(
		"",
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion("1.0.0"),
	)

	// 创建 gRPC 连接
	conn, err := grpc.NewClient(
		endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection: %w", err)
	}

	// 创建 OTLP Exporter
	exporter, err := otlptracegrpc.New(context.Background(),
		otlptracegrpc.WithGRPCConn(conn),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create exporter: %w", err)
	}

	// 创建 TracerProvider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	// 设置全局 TracerProvider
	otel.SetTracerProvider(tp)

	// 设置 Propagator (用于跨服务传播 trace context)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// 获取 Tracer
	tracer = tp.Tracer(serviceName)

	return tp, nil
}

// OllamaChatRequest Ollama 聊天请求
type OllamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []OllamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

// OllamaMessage Ollama 消息
type OllamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OllamaResponse Ollama 响应
type OllamaResponse struct {
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
	Message   struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Done bool `json:"done"`
}

// callJavaService 调用 Java 服务 (演示跨服务调用)
func callJavaService(ctx context.Context, javaURL string) (map[string]interface{}, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "POST", javaURL+"/api/java/tool", bytes.NewBufferString("{}"))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	// 注入 trace context 到 HTTP header
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// callOllama 调用 Ollama 大模型
func callOllama(ctx context.Context, ollamaURL, model, query string) (*OllamaResponse, error) {
	_, span := tracer.Start(ctx, "ollama.chat",
		trace.WithAttributes(
			attribute.String("llm.provider", "ollama"),
			attribute.String("llm.model", model),
			attribute.String("llm.query.length", fmt.Sprintf("%d", len(query))),
		),
	)
	defer span.End()

	client := &http.Client{Timeout: 120 * time.Second}

	reqBody := OllamaChatRequest{
		Model: model,
		Messages: []OllamaMessage{
			{Role: "user", Content: query},
		},
		Stream: false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", ollamaURL+"/api/chat", bytes.NewBuffer(jsonBody))
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	var ollamaResp OllamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(attribute.String("llm.response.length", fmt.Sprintf("%d", len(ollamaResp.Message.Content))))

	return &ollamaResp, nil
}

// checkOllamaAvailable 检查 Ollama 是否可用
func checkOllamaAvailable(ollamaURL string) bool {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(ollamaURL + "/api/tags")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func main() {
	config := loadConfig()

	// 初始化 OpenTelemetry
	tp, err := initOTel(config.ServiceName, config.OtelEndpoint)
	if err != nil {
		log.Printf("Warning: Failed to initialize OpenTelemetry: %v (continuing without tracing)", err)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := tp.Shutdown(ctx); err != nil {
				log.Printf("Error shutting down tracer provider: %v", err)
			}
		}()
	}

	// 创建 Gin 引擎
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.Default()

	// 添加 OpenTelemetry 中间件
	r.Use(otelgin.Middleware(config.ServiceName))

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":           "UP",
			"service":          config.ServiceName,
			"ollama_available": checkOllamaAvailable(config.OllamaURL),
			"time":             time.Now().Format(time.RFC3339),
		})
	})

	// 工具接口 - 被 APISIX 或其他服务调用
	r.POST("/api/go/tool", func(c *gin.Context) {
		ctx := c.Request.Context()
		span := trace.SpanFromContext(ctx)

		span.SetAttributes(
			attribute.String("http.route", "/api/go/tool"),
			attribute.String("service.name", config.ServiceName),
		)

		c.JSON(http.StatusOK, gin.H{
			"data":             "go tool done",
			"service":          config.ServiceName,
			"trace_id":         span.SpanContext().TraceID().String(),
			"ollama_available": checkOllamaAvailable(config.OllamaURL),
		})
	})

	// 调用 Java 服务的接口 - 演示跨服务调用
	r.POST("/api/go/call-java", func(c *gin.Context) {
		ctx := c.Request.Context()
		span := trace.SpanFromContext(ctx)

		span.SetAttributes(attribute.String("http.route", "/api/go/call-java"))

		result, err := callJavaService(ctx, config.JavaServiceURL)
		if err != nil {
			span.RecordError(err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   err.Error(),
				"service": config.ServiceName,
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"java_response": result,
			"service":       config.ServiceName,
			"trace_id":      span.SpanContext().TraceID().String(),
		})
	})

	// 调用 Ollama LLM 的接口
	r.POST("/api/go/chat", func(c *gin.Context) {
		var req struct {
			Query string `json:"query"`
			Model string `json:"model"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}

		if req.Model == "" {
			req.Model = "qwen3:4b"
		}

		ctx := c.Request.Context()
		span := trace.SpanFromContext(ctx)

		span.SetAttributes(
			attribute.String("http.route", "/api/go/chat"),
			attribute.String("query", req.Query),
			attribute.String("model", req.Model),
		)

		startTime := time.Now()

		// 调用 Ollama
		resp, err := callOllama(ctx, config.OllamaURL, req.Model, req.Query)
		duration := time.Since(startTime).Milliseconds()

		if err != nil {
			span.RecordError(err)
			c.JSON(http.StatusOK, gin.H{
				"error":            err.Error(),
				"ollama_available": false,
				"duration_ms":      duration,
				"service":          config.ServiceName,
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"response":    resp.Message.Content,
			"model":       resp.Model,
			"duration_ms": duration,
			"service":     config.ServiceName,
			"trace_id":    span.SpanContext().TraceID().String(),
		})
	})

	// 配置信息接口
	r.GET("/config", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service_name":     config.ServiceName,
			"otel_endpoint":    config.OtelEndpoint,
			"java_service_url": config.JavaServiceURL,
			"ollama_url":       config.OllamaURL,
			"port":             config.Port,
		})
	})

	// 启动服务
	log.Printf("Starting %s on port %s", config.ServiceName, config.Port)
	log.Printf("OpenTelemetry endpoint: %s", config.OtelEndpoint)
	log.Printf("Java service URL: %s", config.JavaServiceURL)
	log.Printf("Ollama URL: %s", config.OllamaURL)

	if err := r.Run(":" + config.Port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
