//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/JunoPayServer/juno-pay-server/internal/scanclient"
	"github.com/JunoPayServer/juno-pay-server/internal/testutil"
	"github.com/JunoPayServer/juno-pay-server/internal/testutil/containers"
)

func TestPayServer_DepositFlow_E2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	jd, err := containers.StartJunocashd(ctx)
	if err != nil {
		t.Fatalf("StartJunocashd: %v", err)
	}
	defer func() { _ = jd.Terminate(context.Background()) }()

	_, merchantUA, merchantUFVK := mustCreateWalletAndUFVK(t, ctx, jd)
	uaHRP := strings.SplitN(merchantUA, "1", 2)[0]
	_, fundUA, fundUFVK := mustCreateWalletAndUFVK(t, ctx, jd)

	scanBin, err := testutil.EnsureTool(ctx, testutil.ToolSpec{
		EnvVar:      "JUNO_TEST_JUNO_SCAN_BIN",
		BinaryName:  "juno-scan",
		SiblingPath: filepath.Join("..", "juno-scan", "bin", "juno-scan"),
		BuildDir:    filepath.Join("..", "juno-scan"),
	})
	if err != nil {
		t.Fatalf("EnsureTool(juno-scan): %v", err)
	}

	scanPort, err := testutil.FreePort()
	if err != nil {
		t.Fatalf("FreePort(scan): %v", err)
	}
	scanURL := "http://127.0.0.1:" + strconv.Itoa(scanPort)

	scanDBPath := filepath.Join(t.TempDir(), "scan-db")
	scanProc, err := testutil.StartProcess(ctx, scanBin, []string{
		"-listen", "127.0.0.1:" + strconv.Itoa(scanPort),
		"-db-driver", "rocksdb",
		"-db-path", scanDBPath,
		"-rpc-url", jd.RPCURL,
		"-rpc-user", jd.RPCUser,
		"-rpc-pass", jd.RPCPassword,
		"-ua-hrp", uaHRP,
		"-poll-interval", "100ms",
		"-confirmations", "100",
	}, nil)
	if err != nil {
		t.Fatalf("StartProcess(juno-scan): %v", err)
	}
	defer func() { _ = scanProc.Terminate(context.Background()) }()

	readyCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := testutil.WaitForHTTP200(readyCtx, scanURL+"/v1/health"); err != nil {
		t.Fatalf("juno-scan not ready: %v\n%s", err, scanProc.Logs())
	}

	payBin := filepath.Join("..", "..", "bin", "juno-pay-server")
	if _, err := os.Stat(payBin); err != nil {
		t.Fatalf("missing binary (run `make build`): %v", err)
	}

	payPort, err := testutil.FreePort()
	if err != nil {
		t.Fatalf("FreePort(pay): %v", err)
	}
	payURL := "http://127.0.0.1:" + strconv.Itoa(payPort)

	var keyRaw [32]byte
	if _, err := rand.Read(keyRaw[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}

	dataDir := filepath.Join(t.TempDir(), "pay")
	env := map[string]string{
		"JUNO_PAY_ADDR":           "127.0.0.1:" + strconv.Itoa(payPort),
		"JUNO_PAY_ADMIN_PASSWORD": "pw",
		"JUNO_PAY_DATA_DIR":       dataDir,
		"JUNO_PAY_TOKEN_KEY_HEX":  hex.EncodeToString(keyRaw[:]),
		"JUNO_CASHD_RPC_URL":      jd.RPCURL,
		"JUNO_CASHD_RPC_USER":     jd.RPCUser,
		"JUNO_CASHD_RPC_PASS":     jd.RPCPassword,
		"JUNO_SCAN_URL":           scanURL,
		"JUNO_PAY_SCAN_POLL_MS":   "200",
	}

	payProc, err := testutil.StartProcess(ctx, payBin, nil, env)
	if err != nil {
		t.Fatalf("StartProcess(juno-pay-server): %v", err)
	}
	defer func() { _ = payProc.Terminate(context.Background()) }()

	if err := testutil.WaitForHTTP200(readyCtx, payURL+"/v1/health"); err != nil {
		t.Fatalf("pay server not ready: %v\n%s", err, payProc.Logs())
	}
	if err := testutil.WaitForHTTP200(readyCtx, payURL+"/v1/status"); err != nil {
		t.Fatalf("pay server status not ready: %v\n%s", err, payProc.Logs())
	}

	client := mustClient(t)
	mustAdminLogin(t, ctx, client, payURL, "pw")

	merchantID := mustAdminCreateMerchant(t, ctx, client, payURL, "acme")
	mustAdminSetWallet(t, ctx, client, payURL, merchantID, "w1", merchantUFVK, uaHRP)
	apiKey := mustAdminCreateAPIKey(t, ctx, client, payURL, merchantID)

	invoiceID, invoiceToken, invoiceAddr := mustCreateInvoice(t, ctx, payURL, apiKey)
	if !strings.HasPrefix(invoiceAddr, uaHRP+"1") {
		t.Fatalf("unexpected invoice address hrp: %q", invoiceAddr)
	}

	// Ensure juno-scan has the wallet before we start generating blocks / deposits.
	mustWaitScannerWallet(t, ctx, scanURL, "w1")

	// Fund a separate account and pay the invoice (prevents overpaying the invoice address via coinbase shielding).
	mustCLI(t, ctx, jd, "generate", "101")
	fromTAddr := mustCoinbaseAddress(t, ctx, jd)
	opid := mustCLIStringOrOpID(t, ctx, jd, "z_shieldcoinbase", fromTAddr, fundUA)
	mustWaitOpSuccess(t, ctx, jd, opid)
	mustCLI(t, ctx, jd, "generate", "2")
	mustWaitOrchardBalanceForViewingKey(t, ctx, jd, fundUFVK, 2)

	opid2 := mustSendMany(t, ctx, jd, fundUA, invoiceAddr, "0.01")
	mustWaitOpSuccess(t, ctx, jd, opid2)
	mustCLI(t, ctx, jd, "generate", "1")

	sc, err := scanclient.New(scanURL)
	if err != nil {
		t.Fatalf("scanclient.New: %v", err)
	}
	mustWaitForScanEventKind(t, ctx, sc, "w1", "DepositEvent")
	mustCLI(t, ctx, jd, "generate", "4") // merchant required_confirmations=5

	mustWaitInvoiceConfirmed(t, ctx, payURL, invoiceID, invoiceToken, 1_000_000)
}

func mustClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	return &http.Client{Jar: jar, Timeout: 5 * time.Second}
}

func mustWaitScannerWallet(t *testing.T, ctx context.Context, scanURL, walletID string) {
	t.Helper()

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(30 * time.Second)
	}

	type wallet struct {
		WalletID string `json:"wallet_id"`
	}
	type resp struct {
		Wallets []wallet `json:"wallets"`
	}

	client := &http.Client{Timeout: 3 * time.Second}
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, scanURL+"/v1/wallets", nil)
		r, err := client.Do(req)
		if err == nil && r.StatusCode == http.StatusOK {
			var out resp
			_ = json.NewDecoder(r.Body).Decode(&out)
			_ = r.Body.Close()
			for _, w := range out.Wallets {
				if w.WalletID == walletID {
					return
				}
			}
		} else if r != nil {
			_ = r.Body.Close()
		}
		time.Sleep(200 * time.Millisecond)
	}

	t.Fatalf("wallet not visible in scanner: %s", walletID)
}

func mustWaitForScanEventKind(t *testing.T, ctx context.Context, sc *scanclient.Client, walletID, kind string) {
	t.Helper()
	if sc == nil {
		t.Fatalf("scanclient is nil")
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(60 * time.Second)
	}

	cursor := int64(0)
	for time.Now().Before(deadline) {
		evs, next, err := sc.ListWalletEvents(ctx, walletID, cursor, 200)
		if err == nil {
			for _, e := range evs {
				if e.Kind == kind {
					return
				}
			}
			cursor = next
		}
		time.Sleep(200 * time.Millisecond)
	}

	t.Fatalf("scanner event not seen: wallet=%s kind=%s", walletID, kind)
}

func mustAdminLogin(t *testing.T, ctx context.Context, c *http.Client, baseURL, password string) {
	t.Helper()
	body := []byte(`{"password":"` + password + `"}`)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/admin/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("admin login: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("admin login status=%d", resp.StatusCode)
	}
}

func mustAdminCreateMerchant(t *testing.T, ctx context.Context, c *http.Client, baseURL, name string) string {
	t.Helper()
	b, _ := json.Marshal(map[string]any{
		"name": name,
		"settings": map[string]any{
			"invoice_ttl_seconds":    900,
			"required_confirmations": 5,
			"policies": map[string]any{
				"late_payment_policy":    "manual_review",
				"partial_payment_policy": "accept_partial",
				"overpayment_policy":     "manual_review",
			},
		},
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/admin/merchants", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create merchant status=%d", resp.StatusCode)
	}
	var out struct {
		Status string `json:"status"`
		Data   struct {
			MerchantID string `json:"merchant_id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Status != "ok" || out.Data.MerchantID == "" {
		t.Fatalf("invalid merchant response: %+v", out)
	}
	return out.Data.MerchantID
}

func mustAdminSetWallet(t *testing.T, ctx context.Context, c *http.Client, baseURL, merchantID, walletID, ufvk, uaHRP string) {
	t.Helper()
	b, _ := json.Marshal(map[string]any{
		"wallet_id": walletID,
		"ufvk":      ufvk,
		"chain":     "regtest",
		"ua_hrp":    uaHRP,
		"coin_type": 8133,
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/admin/merchants/"+merchantID+"/wallet", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("set wallet: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set wallet status=%d", resp.StatusCode)
	}
}

func mustAdminCreateAPIKey(t *testing.T, ctx context.Context, c *http.Client, baseURL, merchantID string) string {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/admin/merchants/"+merchantID+"/api-keys", bytes.NewReader([]byte(`{"label":"default"}`)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create api key status=%d", resp.StatusCode)
	}
	var out struct {
		Status string `json:"status"`
		Data   struct {
			APIKey string `json:"api_key"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Status != "ok" || out.Data.APIKey == "" {
		t.Fatalf("invalid api key response: %+v", out)
	}
	return out.Data.APIKey
}

func mustCreateInvoice(t *testing.T, ctx context.Context, baseURL, apiKey string) (invoiceID, token, addr string) {
	t.Helper()
	body := []byte(`{"external_order_id":"order-1","amount_zat":1000000}`)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/invoices", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create invoice status=%d", resp.StatusCode)
	}
	var out struct {
		Status string `json:"status"`
		Data   struct {
			Invoice struct {
				InvoiceID string `json:"invoice_id"`
				Address   string `json:"address"`
			} `json:"invoice"`
			InvoiceToken string `json:"invoice_token"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Status != "ok" || out.Data.Invoice.InvoiceID == "" || out.Data.Invoice.Address == "" || out.Data.InvoiceToken == "" {
		t.Fatalf("invalid invoice response: %+v", out)
	}
	return out.Data.Invoice.InvoiceID, out.Data.InvoiceToken, out.Data.Invoice.Address
}

func mustWaitInvoiceConfirmed(t *testing.T, ctx context.Context, baseURL, invoiceID, token string, wantConfirmed int64) {
	t.Helper()
	client := &http.Client{Timeout: 3 * time.Second}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(60 * time.Second)
	}
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/public/invoices/"+invoiceID+"?token="+token, nil)
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			var out struct {
				Status string `json:"status"`
				Data   struct {
					Status               string `json:"status"`
					ReceivedConfirmedZat int64  `json:"received_zat_confirmed"`
				} `json:"data"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&out)
			_ = resp.Body.Close()
			if out.Status == "ok" && out.Data.Status == "confirmed" && out.Data.ReceivedConfirmedZat == wantConfirmed {
				return
			}
		} else if resp != nil {
			_ = resp.Body.Close()
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("invoice not confirmed: %s", invoiceID)
}

func mustCreateWalletAndUFVK(t *testing.T, ctx context.Context, jd *containers.Junocashd) (account int, addr string, ufvk string) {
	t.Helper()

	var acc struct {
		Account int `json:"account"`
	}
	mustCLIJSON(t, ctx, jd, &acc, "z_getnewaccount")

	var addrResp struct {
		Address string `json:"address"`
	}
	mustCLIJSON(t, ctx, jd, &addrResp, "z_getaddressforaccount", strconv.Itoa(acc.Account))
	if strings.TrimSpace(addrResp.Address) == "" {
		t.Fatalf("missing address")
	}

	out := mustCLIBytes(t, ctx, jd, "z_exportviewingkey", addrResp.Address)
	ufvk = strings.TrimSpace(string(out))
	if ufvk == "" {
		t.Fatalf("missing ufvk")
	}
	return acc.Account, addrResp.Address, ufvk
}

func mustCoinbaseAddress(t *testing.T, ctx context.Context, jd *containers.Junocashd) string {
	t.Helper()
	out := mustCLIBytes(t, ctx, jd, "listunspent", "1", "9999999")
	var utxos []struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal(out, &utxos); err != nil {
		t.Fatalf("listunspent json: %v\n%s", err, string(out))
	}
	if len(utxos) == 0 || utxos[0].Address == "" {
		t.Fatalf("no utxos found")
	}
	return utxos[0].Address
}

func mustSendMany(t *testing.T, ctx context.Context, jd *containers.Junocashd, fromAddr, toAddr string, amount string) string {
	t.Helper()
	recipients := `[{"address":"` + toAddr + `","amount":` + amount + `}]`
	return mustCLIStringOrOpID(t, ctx, jd, "z_sendmany", fromAddr, recipients, "1")
}

func mustWaitOrchardBalanceForViewingKey(t *testing.T, ctx context.Context, jd *containers.Junocashd, ufvk string, minconf int) int64 {
	t.Helper()

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(60 * time.Second)
	}

	type pool struct {
		ValueZat int64 `json:"valueZat"`
	}
	type resp struct {
		Pools map[string]pool `json:"pools"`
	}

	for time.Now().Before(deadline) {
		out := mustCLIBytes(t, ctx, jd, "z_getbalanceforviewingkey", ufvk, strconv.FormatInt(int64(minconf), 10))
		var r resp
		if err := json.Unmarshal(out, &r); err == nil {
			if p, ok := r.Pools["orchard"]; ok && p.ValueZat > 0 {
				return p.ValueZat
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	t.Fatalf("orchard balance not available for minconf=%d", minconf)
	return 0
}

func mustWaitOpSuccess(t *testing.T, ctx context.Context, jd *containers.Junocashd, opid string) {
	t.Helper()
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(60 * time.Second)
	}
	for time.Now().Before(deadline) {
		out := mustCLIBytes(t, ctx, jd, "z_getoperationresult", `["`+opid+`"]`)
		var res []struct {
			Status string `json:"status"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error,omitempty"`
		}
		if err := json.Unmarshal(out, &res); err == nil && len(res) > 0 {
			switch res[0].Status {
			case "success":
				return
			case "failed":
				msg := ""
				if res[0].Error != nil {
					msg = res[0].Error.Message
				}
				t.Fatalf("operation failed: %s (%s)", opid, msg)
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("operation did not succeed: %s", opid)
}

func mustCLIStringOrOpID(t *testing.T, ctx context.Context, jd *containers.Junocashd, args ...string) string {
	t.Helper()
	out := mustCLIBytes(t, ctx, jd, args...)
	var resp struct {
		OpID string `json:"opid"`
	}
	if err := json.Unmarshal(out, &resp); err == nil && resp.OpID != "" {
		return resp.OpID
	}
	opid := strings.TrimSpace(string(out))
	if opid == "" {
		t.Fatalf("missing opid")
	}
	return opid
}

func mustCLI(t *testing.T, ctx context.Context, jd *containers.Junocashd, args ...string) {
	t.Helper()
	_, _, err := jd.CLI(ctx, args...)
	if err != nil {
		t.Fatalf("junocash-cli %v: %v", args, err)
	}
}

func mustCLIBytes(t *testing.T, ctx context.Context, jd *containers.Junocashd, args ...string) []byte {
	t.Helper()
	stdout, stderr, err := jd.CLI(ctx, args...)
	if err != nil {
		t.Fatalf("junocash-cli %v: %v\nstdout=%s\nstderr=%s", args, err, string(stdout), string(stderr))
	}
	return stdout
}

func mustCLIJSON(t *testing.T, ctx context.Context, jd *containers.Junocashd, out any, args ...string) {
	t.Helper()
	b := mustCLIBytes(t, ctx, jd, args...)
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("junocash-cli json %v: %v\n%s", args, err, string(b))
	}
}
