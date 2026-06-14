package evaluator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// OpenAIEvaluator evaluates tags using an OpenAI-compatible chat completions API.
type OpenAIEvaluator struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewOpenAIEvaluator creates an evaluator backed by an OpenAI-compatible API.
func NewOpenAIEvaluator(apiKey, baseURL, model string) *OpenAIEvaluator {
	slog.Debug("NewOpenAIEvaluator called", "base_url", baseURL, "model", model)
	return &OpenAIEvaluator{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Evaluate evaluates tags for the given content using an LLM.
// For now, only txt data type is supported.
func (e *OpenAIEvaluator) Evaluate(dataType DataType, content []byte, tags []string) (map[string]bool, error) {
	slog.Debug("OpenAIEvaluator.Evaluate", "data_type", dataType, "tags_count", len(tags))
	result := make(map[string]bool, len(tags))
	if dataType != DataTypeTxt {
		slog.Debug("non-txt data type, returning false for all tags")
		for _, tag := range tags {
			result[tag] = false
		}
		return result, nil
	}

	if len(tags) == 0 {
		slog.Debug("no tags provided, returning empty result")
		return result, nil
	}

	prompt := buildPrompt(string(content), tags)
	respBody, err := e.callChatCompletions(prompt)
	if err != nil {
		return nil, err
	}

	llmResult, err := parseTagResponse(respBody, tags)
	if err != nil {
		return nil, err
	}

	return llmResult, nil
}

func buildPrompt(text string, tags []string) string {
	slog.Debug("buildPrompt called", "tags_count", len(tags))
	t := make([]byte, 0, len(tags)*16)
	t = append(t, '[')
	for i, tag := range tags {
		if i > 0 {
			t = append(t, ", "...)
		}
		t = append(t, '"')
		t = append(t, tag...)
		t = append(t, '"')
	}
	t = append(t, ']')
	return fmt.Sprintf(
		`Analyze the following text and determine which tags apply. Respond ONLY with a JSON object where each key is a tag and the value is true or false.

Example response format:
{"tag1": true, "tag2": false}

Text:
---
%s
---

Tags: %s`,
		text,
		string(t),
	)
}

func (e *OpenAIEvaluator) callChatCompletions(prompt string) ([]byte, error) {
	slog.Debug("callChatCompletions called")
	payload := map[string]any{
		"model": e.model,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a tag evaluation engine that responds only with JSON."},
			{"role": "user", "content": prompt},
		},
		"temperature": 0,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal chat completion request: %w", err)
	}

	url := e.baseURL + "/chat/completions"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create chat completion request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chat completion request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read chat completion response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chat completion returned status %d: %s", resp.StatusCode, string(respBytes))
	}

	return respBytes, nil
}

func parseTagResponse(respBody []byte, expectedTags []string) (map[string]bool, error) {
	slog.Debug("parseTagResponse called", "expected_tags_count", len(expectedTags))
	result := make(map[string]bool, len(expectedTags))
	for _, tag := range expectedTags {
		result[tag] = false
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal chat completion response: %w", err)
	}
	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in chat completion response")
	}

	content := apiResp.Choices[0].Message.Content

	// Try to parse the content directly as JSON.
	var parsed map[string]any
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		slog.Debug("failed to parse content as JSON, attempting fallback extraction")
		// Fallback: extract the first JSON object from the text.
		start := bytes.IndexByte([]byte(content), '{')
		end := bytes.LastIndexByte([]byte(content), '}')
		if start == -1 || end == -1 || end <= start {
			return nil, fmt.Errorf("could not extract JSON object from content: %s", content)
		}
		if err := json.Unmarshal([]byte(content[start:end+1]), &parsed); err != nil {
			return nil, fmt.Errorf("unmarshal extracted JSON: %w", err)
		}
	}

	for _, tag := range expectedTags {
		if v, ok := parsed[tag]; ok {
			switch val := v.(type) {
			case bool:
				result[tag] = val
			case string:
				result[tag] = val == "true"
			case float64:
				result[tag] = val != 0
			}
		}
	}

	return result, nil
}

// GetSupportedDataTypes returns the data types supported by this evaluator.
func (e *OpenAIEvaluator) GetSupportedDataTypes() []string {
	slog.Debug("OpenAIEvaluator.GetSupportedDataTypes called")
	return []string{string(DataTypeTxt)}
}
