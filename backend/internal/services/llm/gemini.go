package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// GeminiService handles communication with the Google Gemini API.
type GeminiService struct {
	apiKey string
	client *http.Client
}

// NewGeminiService creates a new Gemini service.
func NewGeminiService() *GeminiService {
	return &GeminiService{
		apiKey: os.Getenv("GEMINI_API_KEY"),
		client: &http.Client{},
	}
}

// GenerateReport creates a Shariah-compliant research report using Gemini 3.1 Pro (simulated as gemini-1.5-pro for REST API compatibility).
func (s *GeminiService) GenerateReport(ctx context.Context, symbol string, technicals, fundamentals, shariah interface{}) (string, error) {
	if s.apiKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY is not set")
	}

	// Prepare data contexts
	techJSON, _ := json.MarshalIndent(technicals, "", "  ")
	fundJSON, _ := json.MarshalIndent(fundamentals, "", "  ")
	sharJSON, _ := json.MarshalIndent(shariah, "", "  ")

	prompt := fmt.Sprintf(`You are a Senior Quantitative Finance Analyst specializing in Shariah-compliant equities on the NSE.
Your task is to write a cohesive, professional research report for the symbol: %s.

CRITICAL RULES:
1. DO NOT INVENT, ESTIMATE, OR CALCULATE ANY NUMBERS. You must strictly use the JSON data provided below.
2. If a metric is missing, do not guess it.
3. Clearly state the Shariah compliance status at the very beginning of the report. If it is "FAIL", issue a strong warning.
4. Keep the tone analytical and objective.

--- TECHNICAL DATA ---
%s

--- FUNDAMENTAL DATA ---
%s

--- SHARIAH DATA ---
%s

Generate a Markdown-formatted report covering:
1. Executive Summary & Shariah Status
2. Fundamental Valuation
3. Technical & Momentum Context
4. Final Deterministic Conclusion (based on data)
`, symbol, string(techJSON), string(fundJSON), string(sharJSON))

	return s.callGeminiAPI(ctx, prompt)
}

func (s *GeminiService) callGeminiAPI(ctx context.Context, prompt string) (string, error) {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-pro:generateContent?key=%s", s.apiKey)

	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature": 0.2, // Low temperature for factual, analytical tone
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal gemini payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gemini api returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode gemini response: %w", err)
	}

	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		return result.Candidates[0].Content.Parts[0].Text, nil
	}

	return "", fmt.Errorf("no content returned from gemini")
}
