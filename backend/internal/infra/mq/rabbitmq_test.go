package inframq

import (
	infraconfig "GCFeed/internal/infra/config"
	"errors"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestRetryCountOfReadsHeader(t *testing.T) {
	cases := []struct {
		name    string
		headers amqp.Table
		want    int
	}{
		{name: "missing", headers: nil, want: 0},
		{name: "int64", headers: amqp.Table{retryCountHeader: int64(2)}, want: 2},
		{name: "int", headers: amqp.Table{retryCountHeader: 4}, want: 4},
		{name: "float64", headers: amqp.Table{retryCountHeader: float64(3)}, want: 3},
		{name: "negative", headers: amqp.Table{retryCountHeader: int64(-1)}, want: 0},
		{name: "wrong type", headers: amqp.Table{retryCountHeader: "2"}, want: 0},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if got := retryCountOf(amqp.Delivery{Headers: item.headers}); got != item.want {
				t.Fatalf("retryCountOf = %d, want %d", got, item.want)
			}
		})
	}
}

func TestMaxRetriesFallsBackToDefault(t *testing.T) {
	cases := []struct {
		name   string
		config infraconfig.RabbitMQConfig
		want   int
	}{
		{name: "zero", config: infraconfig.RabbitMQConfig{}, want: defaultMaxRetries},
		{name: "negative", config: infraconfig.RabbitMQConfig{MaxRetries: -1}, want: defaultMaxRetries},
		{name: "explicit", config: infraconfig.RabbitMQConfig{MaxRetries: 5}, want: 5},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if got := (&RabbitMQ{config: item.config}).maxRetries(); got != item.want {
				t.Fatalf("maxRetries = %d, want %d", got, item.want)
			}
		})
	}
}

func TestNormalizeRabbitMQConfigAppliesDefaults(t *testing.T) {
	cfg := normalizeRabbitMQConfig(infraconfig.RabbitMQConfig{URL: "amqp://guest:guest@localhost:5672/"})
	if cfg.MaxRetries != defaultMaxRetries {
		t.Fatalf("MaxRetries = %d, want %d", cfg.MaxRetries, defaultMaxRetries)
	}
	if cfg.ViewEventRecordedQueue == "" || cfg.ActionChangedQueue == "" {
		t.Fatal("queue names must fall back to defaults")
	}

	cfg = normalizeRabbitMQConfig(infraconfig.RabbitMQConfig{URL: "amqp://guest:guest@localhost:5672/", MaxRetries: 1})
	if cfg.MaxRetries != 1 {
		t.Fatalf("MaxRetries = %d, want 1", cfg.MaxRetries)
	}
}

func TestIsPreconditionFailed(t *testing.T) {
	if isPreconditionFailed(nil) {
		t.Fatal("nil must not be precondition failed")
	}
	if isPreconditionFailed(errors.New("boom")) {
		t.Fatal("plain error must not be precondition failed")
	}
	if !isPreconditionFailed(&amqp.Error{Code: amqp.PreconditionFailed, Reason: "inequivalent arg"}) {
		t.Fatal("amqp precondition error must be detected")
	}
	if isPreconditionFailed(&amqp.Error{Code: amqp.NotFound, Reason: "not found"}) {
		t.Fatal("other amqp errors must not be treated as precondition failed")
	}
}
