package com.example.javaagent.service;

import com.example.javaagent.config.OllamaConfig;
import com.example.javaagent.dto.ChatResponse;
import com.example.javaagent.dto.OllamaRequest;
import com.example.javaagent.dto.OllamaResponse;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.opentelemetry.api.trace.Span;
import io.opentelemetry.api.trace.Tracer;
import io.opentelemetry.context.Scope;
import lombok.extern.slf4j.Slf4j;
import org.springframework.http.HttpHeaders;
import org.springframework.http.MediaType;
import org.springframework.http.client.SimpleClientHttpRequestFactory;
import org.springframework.stereotype.Service;
import org.springframework.web.client.RestClient;

import jakarta.annotation.PostConstruct;
import java.time.Duration;
import java.util.List;

@Slf4j
@Service
public class LLMService {

    private final OllamaConfig ollamaConfig;
    private final Tracer tracer;
    private final ObjectMapper objectMapper;
    private RestClient restClient;

    public LLMService(OllamaConfig ollamaConfig, Tracer tracer, ObjectMapper objectMapper) {
        this.ollamaConfig = ollamaConfig;
        this.tracer = tracer;
        this.objectMapper = objectMapper;
    }

    @PostConstruct
    public void init() {
        SimpleClientHttpRequestFactory factory = new SimpleClientHttpRequestFactory();
        factory.setConnectTimeout(Duration.ofSeconds(10));
        factory.setReadTimeout(Duration.ofMillis(ollamaConfig.getTimeout()));

        this.restClient = RestClient.builder()
                .requestFactory(factory)
                .build();
    }

    /**
     * 调用 Ollama 本地大模型
     */
    public ChatResponse chat(String query, String modelOverride) {
        Span span = tracer.spanBuilder("llm.chat").startSpan();
        long startTime = System.currentTimeMillis();

        try (Scope ignored = span.makeCurrent()) {
            String model = modelOverride != null ? modelOverride : ollamaConfig.getModel();
            span.setAttribute("llm.model", model);
            span.setAttribute("llm.provider", "ollama");
            span.setAttribute("llm.query.length", query != null ? query.length() : 0);

            log.info("Calling Ollama with model: {}, query: {}", model, query);

            OllamaRequest request = OllamaRequest.builder()
                    .model(model)
                    .messages(List.of(
                            OllamaRequest.Message.builder()
                                    .role("user")
                                    .content(query)
                                    .build()
                    ))
                    .stream(false)
                    .build();

            String requestBody = objectMapper.writeValueAsString(request);
            log.debug("Request body: {}", requestBody);

            String responseBody = restClient.post()
                    .uri(ollamaConfig.getBaseUrl() + "/api/chat")
                    .header(HttpHeaders.CONTENT_TYPE, MediaType.APPLICATION_JSON_VALUE)
                    .body(requestBody)
                    .retrieve()
                    .body(String.class);

            log.debug("Response body: {}", responseBody);

            OllamaResponse response = objectMapper.readValue(responseBody, OllamaResponse.class);

            long duration = System.currentTimeMillis() - startTime;
            span.setAttribute("llm.response.duration_ms", duration);

            if (response != null && response.getMessage() != null) {
                log.info("Ollama response received in {}ms", duration);
                return ChatResponse.of(
                        response.getMessage().getContent(),
                        model,
                        duration
                );
            } else {
                span.recordException(new RuntimeException("Empty response from Ollama"));
                throw new RuntimeException("Empty response from Ollama");
            }
        } catch (Exception e) {
            span.recordException(e);
            log.error("Error calling Ollama: {}", e.getMessage(), e);
            throw new RuntimeException("Failed to call Ollama: " + e.getMessage(), e);
        } finally {
            span.end();
        }
    }

    /**
     * 检查 Ollama 服务是否可用
     */
    public boolean isOllamaAvailable() {
        Span span = tracer.spanBuilder("llm.healthcheck").startSpan();
        try {
            String result = restClient.get()
                    .uri(ollamaConfig.getBaseUrl() + "/api/tags")
                    .retrieve()
                    .body(String.class);
            boolean available = result != null && !result.contains("error");
            span.setAttribute("ollama.available", available);
            return available;
        } catch (Exception e) {
            log.warn("Ollama not available: {}", e.getMessage());
            span.setAttribute("ollama.available", false);
            return false;
        } finally {
            span.end();
        }
    }

    /**
     * 获取可用的模型列表
     */
    public String listModels() {
        try {
            return restClient.get()
                    .uri(ollamaConfig.getBaseUrl() + "/api/tags")
                    .retrieve()
                    .body(String.class);
        } catch (Exception e) {
            log.error("Failed to list models: {}", e.getMessage());
            return "{\"error\": \"" + e.getMessage() + "\"}";
        }
    }
}