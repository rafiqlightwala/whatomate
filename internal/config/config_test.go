package config

import "testing"

func TestMapEnvKey(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"DATABASE_HOST", "database.host"},
		{"APP_ENCRYPTION__KEY", "app.encryption_key"},
		{"WHATSAPP_WEBHOOK__VERIFY__TOKEN", "whatsapp.webhook_verify_token"},
		{"RATE__LIMIT_TRUST__PROXY", "rate_limit.trust_proxy"},
	}

	for _, tt := range tests {
		got := mapEnvKey(tt.in)
		if got != tt.want {
			t.Fatalf("mapEnvKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

