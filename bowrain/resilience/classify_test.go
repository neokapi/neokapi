package resilience

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// bedrockDailyQuota is the shape the AWS SDK gives a Bedrock call refused by
// the account's tokens-per-day quota, after its own retries.
const bedrockDailyQuota = "bedrock: converse: operation error Bedrock Runtime: Converse, " +
	"exceeded maximum number of attempts, 3, https response error StatusCode: 429, " +
	"RequestID: 0f1e, ThrottlingException: Too many tokens per day, please wait before trying again."

const bedrockThrottled = "bedrock: converse: operation error Bedrock Runtime: Converse, " +
	"https response error StatusCode: 429, RequestID: 0f1e, " +
	"ThrottlingException: Too many tokens, please wait before trying again."

func TestClassifyProviderLimit(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want ProviderLimit
	}{
		{name: "bedrock daily token quota", msg: bedrockDailyQuota, want: ProviderQuotaExhausted},
		{name: "bedrock service quota", msg: "ServiceQuotaExceededException: quota reached", want: ProviderQuotaExhausted},
		{name: "openai insufficient quota", msg: "openai: API error 429: insufficient_quota", want: ProviderQuotaExhausted},
		{name: "openai current quota", msg: "openai: API error 429: You exceeded your current quota", want: ProviderQuotaExhausted},
		{name: "anthropic credit balance", msg: "anthropic: API error 400: Your credit balance is too low", want: ProviderQuotaExhausted},
		{name: "job row after skip", msg: "skipped: the AI provider's quota is exhausted (" + bedrockDailyQuota + ")", want: ProviderQuotaExhausted},

		{name: "bedrock per-minute throttle", msg: bedrockThrottled, want: ProviderThrottled},
		{name: "429 rate limit", msg: "anthropic: API error 429: rate limit exceeded", want: ProviderThrottled},
		{name: "too many requests", msg: "gemini: Too Many Requests", want: ProviderThrottled},

		{name: "503 outage", msg: "openai: API error 503: service unavailable", want: NoProviderLimit},
		{name: "401 bad key", msg: "openai: API error 401: invalid api key", want: NoProviderLimit},
		{name: "item failure", msg: "store blocks: constraint violation", want: NoProviderLimit},
		{name: "workspace abuse cap", msg: "workspace AI quota exceeded", want: NoProviderLimit},
		{name: "empty", msg: "", want: NoProviderLimit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ClassifyProviderLimit(tt.msg))
		})
	}
}

func TestIsUpstreamFailure_ProviderLimits(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "per-minute throttle is retried", err: errors.New(bedrockThrottled), want: true},
		{name: "daily quota is not retried", err: errors.New(bedrockDailyQuota), want: false},
		{name: "wrapped daily quota is not retried", err: fmt.Errorf("translate chunk 0-50: %w", errors.New(bedrockDailyQuota)), want: false},
		{name: "openai insufficient quota is not retried", err: errors.New("openai: API error 429: insufficient_quota"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsUpstreamFailure(tt.err))
			assert.Equal(t, !tt.want, IsQuotaExhausted(tt.err))
		})
	}
}
