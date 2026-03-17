package com.example.javaagent.dto;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

/**
 * 聊天响应
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class ChatResponse {

    private String response;
    private String model;
    private long durationMs;

    public static ChatResponse of(String response, String model, long durationMs) {
        return ChatResponse.builder()
                .response(response)
                .model(model)
                .durationMs(durationMs)
                .build();
    }
}