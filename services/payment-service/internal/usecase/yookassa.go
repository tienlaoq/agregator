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
	shopID    string
	secretKey string
	httpClient *http.Client
	mockMode  bool
}

func NewYooKassaClient(shopID, secretKey string) *YooKassaClient {
	return &YooKassaClient{
		shopID:     shopID,
		secretKey:  secretKey,
		httpClient: &http.Client{},
		mockMode:   shopID == "",
	}
}

type yooKassaRequest struct {
	Amount       yooKassaAmount       `json:"amount"`
	Confirmation yooKassaConfirmation `json:"confirmation"`
	Description  string               `json:"description"`
	Capture      bool                 `json:"capture"`
}

type yooKassaAmount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

type yooKassaConfirmation struct {
	Type      string `json:"type"`
	ReturnURL string `json:"return_url"`
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

func (c *YooKassaClient) CreatePayment(amount int64, description, returnURL, idempotencyKey string) (*YooKassaPaymentResult, error) {
	if c.mockMode {
		mockID := uuid.New().String()
		return &YooKassaPaymentResult{
			PaymentID:       mockID,
			ConfirmationURL: fmt.Sprintf("https://mock.yookassa.ru/pay/%s", mockID),
		}, nil
	}

	body := yooKassaRequest{
		Amount: yooKassaAmount{
			Value:    fmt.Sprintf("%.2f", float64(amount)/100),
			Currency: "RUB",
		},
		Confirmation: yooKassaConfirmation{
			Type:      "redirect",
			ReturnURL: returnURL,
		},
		Description: description,
		Capture:     true,
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
