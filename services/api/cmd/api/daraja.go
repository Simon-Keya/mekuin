package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// darajaClient is deliberately isolated from handlers so provider behaviour can
// be tested without credentials or a live Safaricom account.
type darajaClient struct {
	baseURL, consumerKey, consumerSecret, shortcode, passkey, callbackURL string
	timeout                                                               time.Duration
	httpClient                                                            *http.Client
}

type darajaSTKRequest struct {
	BusinessShortCode string `json:"BusinessShortCode"`
	Password          string `json:"Password"`
	Timestamp         string `json:"Timestamp"`
	TransactionType   string `json:"TransactionType"`
	Amount            int64  `json:"Amount"`
	PartyA            string `json:"PartyA"`
	PartyB            string `json:"PartyB"`
	PhoneNumber       string `json:"PhoneNumber"`
	CallBackURL       string `json:"CallBackURL"`
	AccountReference  string `json:"AccountReference"`
	TransactionDesc   string `json:"TransactionDesc"`
}

type darajaSTKResponse struct {
	MerchantRequestID   string `json:"MerchantRequestID"`
	CheckoutRequestID   string `json:"CheckoutRequestID"`
	ResponseCode        string `json:"ResponseCode"`
	ResponseDescription string `json:"ResponseDescription"`
	CustomerMessage     string `json:"CustomerMessage"`
}

type darajaSTKQueryRequest struct {
	BusinessShortCode string `json:"BusinessShortCode"`
	Password          string `json:"Password"`
	Timestamp         string `json:"Timestamp"`
	CheckoutRequestID string `json:"CheckoutRequestID"`
}

type darajaSTKQueryResponse struct {
	ResponseCode        string `json:"ResponseCode"`
	ResponseDescription string `json:"ResponseDescription"`
	MerchantRequestID   string `json:"MerchantRequestID"`
	CheckoutRequestID   string `json:"CheckoutRequestID"`
	ResultCode          string `json:"ResultCode"`
	ResultDesc          string `json:"ResultDesc"`
}

// querySTK checks the provider-side state for a previously accepted STK request.
// A successful HTTP response is not itself proof of payment: callers must
// inspect ResultCode and independently match the stored payment attempt.
func (d *darajaClient) querySTK(ctx context.Context, checkoutRequestID string, now time.Time) (darajaSTKQueryResponse, error) {
	var out darajaSTKQueryResponse
	checkoutRequestID = strings.TrimSpace(checkoutRequestID)
	if checkoutRequestID == "" || len(checkoutRequestID) > 128 {
		return out, errors.New("invalid CheckoutRequestID")
	}
	token, err := d.accessToken(ctx)
	if err != nil {
		return out, err
	}
	timestamp := now.UTC().Format("20060102150405")
	password := base64.StdEncoding.EncodeToString([]byte(d.shortcode + d.passkey + timestamp))
	payload, err := json.Marshal(darajaSTKQueryRequest{BusinessShortCode: d.shortcode, Password: password, Timestamp: timestamp, CheckoutRequestID: checkoutRequestID})
	if err != nil {
		return out, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+"/mpesa/stkpushquery/v1/query", bytes.NewReader(payload))
	if err != nil {
		return out, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, fmt.Errorf("Daraja STK query returned HTTP %d", resp.StatusCode)
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return out, err
	}
	if out.ResponseCode != "0" {
		return out, errors.New("Daraja did not accept the STK query")
	}
	if out.CheckoutRequestID != "" && out.CheckoutRequestID != checkoutRequestID {
		return out, errors.New("Daraja STK query returned a mismatched CheckoutRequestID")
	}
	return out, nil
}

func newDarajaClientFromEnv() (*darajaClient, error) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("DARAJA_ENV")))
	base := "https://sandbox.safaricom.co.ke"
	if mode == "production" {
		base = "https://api.safaricom.co.ke"
	} else if mode != "" && mode != "sandbox" {
		return nil, errors.New("DARAJA_ENV must be sandbox or production")
	}
	if v := strings.TrimSpace(os.Getenv("DARAJA_BASE_URL")); v != "" {
		u, err := url.Parse(v)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return nil, errors.New("DARAJA_BASE_URL must be an https URL")
		}
		base = strings.TrimRight(v, "/")
	}
	c := &darajaClient{baseURL: base, consumerKey: os.Getenv("DARAJA_CONSUMER_KEY"), consumerSecret: os.Getenv("DARAJA_CONSUMER_SECRET"), shortcode: os.Getenv("DARAJA_SHORTCODE"), passkey: os.Getenv("DARAJA_PASSKEY"), callbackURL: os.Getenv("DARAJA_CALLBACK_URL"), timeout: 12 * time.Second}
	for _, v := range []string{c.consumerKey, c.consumerSecret, c.shortcode, c.passkey, c.callbackURL} {
		if strings.TrimSpace(v) == "" {
			return nil, errors.New("Daraja credentials and callback URL must be configured")
		}
	}
	cb, err := url.Parse(c.callbackURL)
	if err != nil || cb.Scheme != "https" || cb.Host == "" {
		return nil, errors.New("DARAJA_CALLBACK_URL must be an https URL")
	}
	c.httpClient = &http.Client{Timeout: c.timeout}
	return c, nil
}

func (d *darajaClient) accessToken(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.baseURL+"/oauth/v1/generate?grant_type=client_credentials", nil)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(d.consumerKey, d.consumerSecret)
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   string `json:"expires_in"`
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Daraja OAuth returned HTTP %d", resp.StatusCode)
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return "", err
	}
	if body.AccessToken == "" {
		return "", errors.New("Daraja OAuth response omitted access_token")
	}
	return body.AccessToken, nil
}

func (d *darajaClient) initiateSTK(ctx context.Context, phone string, amount int64, reference, description string, now time.Time) (darajaSTKResponse, error) {
	var out darajaSTKResponse
	if amount < 1 || amount > 250000 || !validDarajaPhone(phone) {
		return out, errors.New("invalid STK amount or phone number")
	}
	token, err := d.accessToken(ctx)
	if err != nil {
		return out, err
	}
	timestamp := now.UTC().Format("20060102150405")
	password := base64.StdEncoding.EncodeToString([]byte(d.shortcode + d.passkey + timestamp))
	payload := darajaSTKRequest{BusinessShortCode: d.shortcode, Password: password, Timestamp: timestamp, TransactionType: "CustomerPayBillOnline", Amount: amount, PartyA: phone, PartyB: d.shortcode, PhoneNumber: phone, CallBackURL: d.callbackURL, AccountReference: boundedText(reference, 12), TransactionDesc: boundedText(description, 13)}
	b, err := json.Marshal(payload)
	if err != nil {
		return out, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+"/mpesa/stkpush/v1/processrequest", bytes.NewReader(b))
	if err != nil {
		return out, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, fmt.Errorf("Daraja STK Push returned HTTP %d", resp.StatusCode)
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return out, err
	}
	if out.ResponseCode != "0" || out.CheckoutRequestID == "" || out.MerchantRequestID == "" {
		return out, errors.New("Daraja did not accept the STK Push request")
	}
	return out, nil
}

func validDarajaPhone(v string) bool {
	if len(v) != 12 || !strings.HasPrefix(v, "254") {
		return false
	}
	for _, r := range v {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
func boundedText(v string, n int) string {
	v = strings.TrimSpace(v)
	if len(v) > n {
		v = v[:n]
	}
	return v
}
