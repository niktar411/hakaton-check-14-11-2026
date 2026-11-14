package operations

import "testing"

func TestValidateModelEndpointOnlyAllowsDeepSeekFlash(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		model   string
		wantErr bool
	}{
		{name: "cloud flash", url: "https://foundation-models.api.cloud.ru/v1", model: "deepseek-ai/DeepSeek-V4.1-Flash"},
		{name: "opencode flash", url: "https://opencode.ai/inference/go/openai/v1", model: "deepseek-v4.1-flash"},
		{name: "cloud other model", url: "https://foundation-models.api.cloud.ru/v1", model: "DeepSeek-V3", wantErr: true},
		{name: "opencode other model", url: "https://opencode.ai/inference/go/openai/v1", model: "deepseek-v4-pro", wantErr: true},
		{name: "cloud model on opencode", url: "https://opencode.ai/inference/go/openai/v1", model: "deepseek-ai/DeepSeek-V4.1-Flash", wantErr: true},
		{name: "unknown host", url: "https://example.com/v1", model: "deepseek-v4.1-flash", wantErr: true},
		{name: "plain http", url: "http://opencode.ai/inference/go/openai/v1", model: "deepseek-v4.1-flash", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateModelEndpoint(test.url, test.model)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateModelEndpoint(%q, %q) = %v, wantErr %v", test.url, test.model, err, test.wantErr)
			}
		})
	}
}
