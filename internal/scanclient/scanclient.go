package scanclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string) (*Client, error) {
	baseURL = strings.TrimSpace(baseURL)
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		return nil, errors.New("scanclient: base url is required")
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, errors.New("scanclient: invalid base url")
	}
	return &Client{
		baseURL: baseURL,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

func (c *Client) Healthy(ctx context.Context) (bool, error) {
	if c == nil || c.baseURL == "" || c.http == nil {
		return false, errors.New("scanclient: not initialized")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/health", nil)
	if err != nil {
		return false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, nil
	}
	var out struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false, err
	}
	return strings.TrimSpace(out.Status) == "ok", nil
}

type walletUpsertRequest struct {
	WalletID string `json:"wallet_id"`
	UFVK     string `json:"ufvk"`
}

func (c *Client) UpsertWallet(ctx context.Context, walletID, ufvk string) error {
	walletID = strings.TrimSpace(walletID)
	ufvk = strings.TrimSpace(ufvk)
	if walletID == "" || ufvk == "" {
		return errors.New("scanclient: wallet_id and ufvk are required")
	}

	body, _ := json.Marshal(walletUpsertRequest{WalletID: walletID, UFVK: ufvk})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/wallets", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("scanclient: upsert wallet: http %d", resp.StatusCode)
	}
	return nil
}

type WalletEvent struct {
	ID        int64           `json:"id"`
	Kind      string          `json:"kind"`
	WalletID  string          `json:"wallet_id"`
	Height    int64           `json:"height"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

func (c *Client) ListWalletEvents(ctx context.Context, walletID string, afterID int64, limit int) (events []WalletEvent, nextCursor int64, err error) {
	walletID = strings.TrimSpace(walletID)
	if walletID == "" {
		return nil, 0, errors.New("scanclient: wallet_id is required")
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	u := c.baseURL + "/v1/wallets/" + url.PathEscape(walletID) + "/events?cursor=" + strconv.FormatInt(afterID, 10) + "&limit=" + strconv.Itoa(limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("scanclient: list wallet events: http %d", resp.StatusCode)
	}

	var out struct {
		Events     []WalletEvent `json:"events"`
		NextCursor int64         `json:"next_cursor"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, 0, err
	}
	return out.Events, out.NextCursor, nil
}
