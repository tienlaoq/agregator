package usecase

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
)

type YooKassaClient struct {
	shopID     string
	secretKey  string
	httpClient *http.Client
	mockMode   bool
}

func NewYooKassaClient(shopID, secretKey string) *YooKassaClient {
	return &YooKassaClient{
		shopID:     shopID,
		secretKey:  secretKey,
		httpClient: &http.Client{},
		mockMode:   shopID == "",
	}
}

type yooKassaAmount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

type yooKassaConfirmation struct {
	Type      string `json:"type"`
	ReturnURL string `json:"return_url"`
}

type yooKassaTransfer struct {
	AccountID         string       `json:"account_id"`
	Amount            yooKassaAmount `json:"amount"`
	PlatformFeeAmount yooKassaAmount `json:"platform_fee_amount"`
	Description       string       `json:"description,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

type yooKassaRequest struct {
	Amount       yooKassaAmount        `json:"amount"`
	Confirmation yooKassaConfirmation  `json:"confirmation"`
	Description  string                `json:"description"`
	Capture      bool                  `json:"capture"`
	Transfers    []yooKassaTransfer    `json:"transfers,omitempty"`
}

type yooKassaResponse struct {
	ID           string `json:"id"`
	Confirmation struct {
		ConfirmationURL string `json:"confirmation_url"`
	} `json:"confirmation"`
	Status string `json:"status"`
}

type YooKassaPaymentResult struct {
	PaymentID       string
	ConfirmationURL string
}

type SplitTransferParams struct {
	GrossKopecks          int64
	PlatformFeeKopecks    int64
	SellerAccountID       string
	TransferDescription   string
	Metadata              map[string]string
}

func kopecksToAmountStr(k int64) string {
	return fmt.Sprintf("%.2f", float64(k)/100)
}

// CreatePaymentSimple charges the full amount to the shop (no split). Uses immediate capture.
func (c *YooKassaClient) CreatePaymentSimple(amount int64, description, returnURL, idempotencyKey string) (*YooKassaPaymentResult, error) {
	body := yooKassaRequest{
		Amount: yooKassaAmount{
			Value:    kopecksToAmountStr(amount),
			Currency: "RUB",
		},
		Confirmation: yooKassaConfirmation{
			Type:      "redirect",
			ReturnURL: returnURL,
		},
		Description: description,
		Capture:     true,
	}
	return c.postPayment(body, idempotencyKey)
}

// CreatePaymentSplit creates a marketplace split payment (ЮKassa transfers). Uses capture=false; call CapturePayment after authorization.
func (c *YooKassaClient) CreatePaymentSplit(returnURL, idempotencyKey string, orderDescription string, split SplitTransferParams) (*YooKassaPaymentResult, error) {
	if split.SellerAccountID == "" {
		return nil, fmt.Errorf("split payment: empty seller account_id")
	}
	if split.GrossKopecks <= 0 {
		return nil, fmt.Errorf("split payment: non-positive gross")
	}
	if split.PlatformFeeKopecks < 0 || split.PlatformFeeKopecks > split.GrossKopecks {
		return nil, fmt.Errorf("split payment: invalid platform fee")
	}

	body := yooKassaRequest{
		Amount: yooKassaAmount{
			Value:    kopecksToAmountStr(split.GrossKopecks),
			Currency: "RUB",
		},
		Confirmation: yooKassaConfirmation{
			Type:      "redirect",
			ReturnURL: returnURL,
		},
		Description: orderDescription,
		Capture:     false,
		Transfers: []yooKassaTransfer{
			{
				AccountID: split.SellerAccountID,
				Amount: yooKassaAmount{
					Value:    kopecksToAmountStr(split.GrossKopecks),
					Currency: "RUB",
				},
				PlatformFeeAmount: yooKassaAmount{
					Value:    kopecksToAmountStr(split.PlatformFeeKopecks),
					Currency: "RUB",
				},
				Description: split.TransferDescription,
				Metadata:    split.Metadata,
			},
		},
	}
	return c.postPayment(body, idempotencyKey)
}

func (c *YooKassaClient) postPayment(body yooKassaRequest, idempotencyKey string) (*YooKassaPaymentResult, error) {
	if c.mockMode {
		mockID := uuid.New().String()
		return &YooKassaPaymentResult{
			PaymentID:       mockID,
			ConfirmationURL: fmt.Sprintf("https://mock.yookassa.ru/pay/%s", mockID),
		}, nil
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.yookassa.ru/v3/payments", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth(c.shopID, c.secretKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotence-Key", idempotencyKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("yookassa returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result yooKassaResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &YooKassaPaymentResult{
		PaymentID:       result.ID,
		ConfirmationURL: result.Confirmation.ConfirmationURL,
	}, nil
}

// CapturePayment confirms a split payment after the user authorized it (full capture, same split as at creation).
func (c *YooKassaClient) CapturePayment(providerPaymentID, idempotencyKey string) error {
	if c.mockMode {
		return nil
	}
	req, err := http.NewRequest("POST", "https://api.yookassa.ru/v3/payments/"+providerPaymentID+"/capture", bytes.NewReader([]byte("{}")))
	if err != nil {
		return fmt.Errorf("create capture request: %w", err)
	}
	req.SetBasicAuth(c.shopID, c.secretKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotence-Key", idempotencyKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("capture do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("capture read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("yookassa capture returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
