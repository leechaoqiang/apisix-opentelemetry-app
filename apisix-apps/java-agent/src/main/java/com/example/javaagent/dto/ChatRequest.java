package com.example.javaagent.dto;

import lombok.AllArgsConstructor;
import lombok.Data;
import lombok.NoArgsConstructor;

/**
 * 聊天请求
 */
@Data
@NoArgsConstructor
@AllArgsConstructor
public class ChatRequest {

    private String query;
    private String model;  // 可选，指定模型
}