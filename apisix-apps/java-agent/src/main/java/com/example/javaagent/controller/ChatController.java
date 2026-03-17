package com.example.javaagent.controller;

import com.example.javaagent.dto.ChatRequest;
import com.example.javaagent.dto.ChatResponse;
import com.example.javaagent.service.LLMService;
import io.opentelemetry.api.trace.Span;
import io.opentelemetry.api.trace.Tracer;
import io.opentelemetry.context.Scope;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import java.util.HashMap;
import java.util.Map;

@Slf4j
@RestController
@RequestMapping("/api/java")
@RequiredArgsConstructor
public class ChatController {

    private final LLMService llmService;
    private final Tracer tracer;

    /**
     * 聊天接口 - 调用 Ollama 本地大模型
     * POST /api/java/chat
     */
    @PostMapping("/chat")
    public ResponseEntity<ChatResponse> chat(@RequestBody ChatRequest request) {
        Span span = tracer.spanBuilder("controller.chat").startSpan();
        try (Scope ignored = span.makeCurrent()) {
            span.setAttribute("http.route", "/api/java/chat");
            span.setAttribute("query.length", request.getQuery() != null ? request.getQuery().length() : 0);

            log.info("Received chat request: {}", request.getQuery());

            ChatResponse response = llmService.chat(request.getQuery(), request.getModel());
            return ResponseEntity.ok(response);
        } finally {
            span.end();
        }
    }

    /**
     * 简单工具接口 - 用于测试链路追踪
     * POST /api/java/tool
     */
    @PostMapping("/tool")
    public ResponseEntity<Map<String, Object>> tool(@RequestBody(required = false) Map<String, Object> body) {
        Span span = tracer.spanBuilder("controller.tool").startSpan();
        try (Scope ignored = span.makeCurrent()) {
            span.setAttribute("http.route", "/api/java/tool");

            Map<String, Object> response = new HashMap<>();
            response.put("data", "java tool done");
            response.put("input", body);
            response.put("ollama_available", llmService.isOllamaAvailable());
            response.put("trace_id", span.getSpanContext().getTraceId());

            return ResponseEntity.ok(response);
        } finally {
            span.end();
        }
    }

    /**
     * 健康检查
     */
    @GetMapping("/health")
    public ResponseEntity<Map<String, Object>> health() {
        Map<String, Object> status = new HashMap<>();
        status.put("status", "UP");
        status.put("service", "java-agent");
        status.put("ollama_available", llmService.isOllamaAvailable());
        return ResponseEntity.ok(status);
    }

    /**
     * 获取 Ollama 可用模型列表
     */
    @GetMapping("/models")
    public ResponseEntity<String> models() {
        return ResponseEntity.ok(llmService.listModels());
    }
}