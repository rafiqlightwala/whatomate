package handlers

import "testing"

func TestApplyOpenAILimits(t *testing.T) {
	cases := []struct {
		model         string
		wantReasoning bool
	}{
		{"gpt-6-luna", true},
		{"gpt-6.1-sol", true},
		{"gpt-5.4-mini", true},
		{"o3-mini", true},
		{" GPT-6-Luna ", true},
		{"gpt-4.1-mini", false},
		{"gpt-4o", false},
		{"gpt-3.5-turbo", false},
	}
	for _, tc := range cases {
		payload := map[string]any{}
		applyOpenAILimits(payload, tc.model, 1000, 0.7, "none")
		if tc.wantReasoning {
			if payload["max_completion_tokens"] != 1000 || payload["reasoning_effort"] != "none" {
				t.Errorf("%q: want max_completion_tokens + reasoning_effort, got %v", tc.model, payload)
			}
			if _, ok := payload["max_tokens"]; ok {
				t.Errorf("%q: max_tokens must not be sent", tc.model)
			}
			if _, ok := payload["temperature"]; ok {
				t.Errorf("%q: temperature must not be sent", tc.model)
			}
		} else {
			if payload["max_tokens"] != 1000 || payload["temperature"] != 0.7 {
				t.Errorf("%q: want max_tokens + temperature, got %v", tc.model, payload)
			}
			if _, ok := payload["max_completion_tokens"]; ok {
				t.Errorf("%q: max_completion_tokens must not be sent", tc.model)
			}
		}
	}

	payload := map[string]any{}
	applyOpenAILimits(payload, "gpt-4o", 500, 0, "none")
	if _, ok := payload["temperature"]; ok {
		t.Errorf("zero temperature should leave the provider default, got %v", payload)
	}
}
