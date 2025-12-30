package outbox

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Abdullah1738/juno-pay-server/internal/domain"
	"github.com/Abdullah1738/juno-pay-server/internal/store"
)

type Option func(*Worker) error

func WithHTTPClient(c *http.Client) Option {
	return func(w *Worker) error {
		if c == nil {
			return errors.New("outbox: http client is nil")
		}
		w.http = c
		return nil
	}
}

func WithNowFunc(now func() time.Time) Option {
	return func(w *Worker) error {
		if now == nil {
			return errors.New("outbox: now func is nil")
		}
		w.now = now
		return nil
	}
}

func WithPollInterval(d time.Duration) Option {
	return func(w *Worker) error {
		if d <= 0 {
			return errors.New("outbox: poll interval must be > 0")
		}
		w.pollInterval = d
		return nil
	}
}

func WithBatchSize(n int) Option {
	return func(w *Worker) error {
		if n <= 0 || n > 1000 {
			return errors.New("outbox: batch size must be 1..1000")
		}
		w.batchSize = n
		return nil
	}
}

func WithMaxAttempts(n int32) Option {
	return func(w *Worker) error {
		if n <= 0 || n > 1000 {
			return errors.New("outbox: max attempts must be 1..1000")
		}
		w.maxAttempts = n
		return nil
	}
}

type Worker struct {
	st store.Store

	http *http.Client
	now  func() time.Time

	pollInterval time.Duration
	batchSize    int
	maxAttempts  int32
}

func New(st store.Store, opts ...Option) (*Worker, error) {
	if st == nil {
		return nil, errors.New("outbox: store is nil")
	}

	w := &Worker{
		st:           st,
		http:         &http.Client{Timeout: 10 * time.Second},
		now:          func() time.Time { return time.Now().UTC() },
		pollInterval: 500 * time.Millisecond,
		batchSize:    100,
		maxAttempts:  25,
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(w); err != nil {
			return nil, err
		}
	}
	return w, nil
}

func (w *Worker) Sync(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	now := w.now()
	tasks, err := w.st.ListDueDeliveries(ctx, now, w.batchSize)
	if err != nil {
		return err
	}

	for _, t := range tasks {
		attempt := t.Delivery.Attempt + 1
		if attempt < 1 {
			attempt = 1
		}

		if err := w.deliver(ctx, t.Sink, t.Event); err != nil {
			lastErr := err.Error()
			if attempt >= w.maxAttempts {
				if err := w.st.UpdateEventDelivery(ctx, t.Delivery.DeliveryID, domain.EventDeliveryFailed, attempt, nil, &lastErr); err != nil {
					return err
				}
				continue
			}
			next := now.Add(backoff(attempt))
			if err := w.st.UpdateEventDelivery(ctx, t.Delivery.DeliveryID, domain.EventDeliveryPending, attempt, &next, &lastErr); err != nil {
				return err
			}
			continue
		}

		if err := w.st.UpdateEventDelivery(ctx, t.Delivery.DeliveryID, domain.EventDeliveryDelivered, attempt, nil, nil); err != nil {
			return err
		}
	}

	return nil
}

func (w *Worker) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		if err := w.Sync(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func backoff(attempt int32) time.Duration {
	if attempt <= 1 {
		return 1 * time.Second
	}
	// 1s, 2s, 4s, 8s, ... capped at 1m.
	d := time.Second << (attempt - 1)
	if d > time.Minute {
		d = time.Minute
	}
	return d
}

func (w *Worker) deliver(ctx context.Context, sink domain.EventSink, ev domain.CloudEvent) error {
	switch sink.Kind {
	case domain.EventSinkWebhook:
		return w.deliverWebhook(ctx, sink, ev)
	default:
		return fmt.Errorf("unsupported sink kind: %s", sink.Kind)
	}
}

type webhookConfig struct {
	URL       string `json:"url"`
	Secret    string `json:"secret"`
	TimeoutMS int    `json:"timeout_ms"`
}

func (w *Worker) deliverWebhook(ctx context.Context, sink domain.EventSink, ev domain.CloudEvent) error {
	var cfg webhookConfig
	if err := json.Unmarshal(sink.Config, &cfg); err != nil {
		return fmt.Errorf("webhook config invalid json")
	}
	cfg.URL = strings.TrimSpace(cfg.URL)
	if cfg.URL == "" {
		return fmt.Errorf("webhook url is required")
	}
	u, err := url.Parse(cfg.URL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("webhook url invalid")
	}

	body, err := json.Marshal(ev)
	if err != nil {
		return err
	}

	client := w.http
	if cfg.TimeoutMS > 0 {
		timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
		if timeout < 100*time.Millisecond {
			timeout = 100 * time.Millisecond
		}
		if timeout > 60*time.Second {
			timeout = 60 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/cloudevents+json")
	req.Header.Set("User-Agent", "juno-pay-server")

	if cfg.Secret != "" {
		mac := hmac.New(sha256.New, []byte(cfg.Secret))
		_, _ = mac.Write(body)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Juno-Signature", "sha256="+sig)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		return nil
	}

	msg := "http " + resp.Status
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
	if s := strings.TrimSpace(string(b)); s != "" {
		msg += ": " + s
	}
	return errors.New(msg)
}

