package innpark

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

const (
	PAYMENT_TYPE_PAYMENT          = "payment"
	PAYMENT_TYPE_PREAUTHORIZATION = "preauthorization"
)

type PayableMetadata struct {
	Type          string `json:"type"`
	LocationType  string `json:"location_type"`
	LocationId    string `json:"location_id"`
	LocationName  string `json:"location_name"`
	ClusterName   string `json:"cluster_name"`
	ClusterId     string `json:"cluster_id"`
	VehicleId     string `json:"vehicle_id"`
	VehiclePlate  string `json:"vehicle_plate"`
	VehicleName   string `json:"vehicle_name"`
	StartDateTime string `json:"start_date_time"`
	EndDateTime   string `json:"end_date_time"`
	Code          string `json:"code"`
	PlanId        string `json:"plan_id"`
	PlanName      string `json:"plan_name"`
	CreatedAt     string `json:"created_at"`
}

type Payable interface {
	GetId() string
	GetAmount() int
	GetUserId() string
	GetMetadata(core.App) PayableMetadata
}

type Payee interface {
	GetTpvId() string
	GetOrganizationId() string
}

type Payment interface {
	GetApiPaymentId() string
	GetPayableId() string
}

var apiUrl = os.Getenv("API_PAYMENT")
var apiToken = os.Getenv("API_PAYMENT_TOKEN")

func CreateService(payable Payable, payee Payee) error {

	body := strings.NewReader(fmt.Sprintf(`{
		"organization_id": "%s",
		"user_id": "%s",
		"service_id": "%s",
		"amount": %d
	}`, payee.GetOrganizationId(), payable.GetUserId(), payable.GetId(), payable.GetAmount()))

	_, err := makeRequest("POST", apiUrl+"/v1/services/create", body)

	return err
}

func RefundPartialPaymentFromService(payable Payable, amount int) error {
	_, err := RefundPartialPaymentFromServiceWithIdempotencyKey(payable, amount, "")
	return err
}

// RefundPartialPaymentFromServiceWithIdempotencyKey refunds part of a service and
// returns the Payment API result. The key is forwarded unchanged when provided.
func RefundPartialPaymentFromServiceWithIdempotencyKey(payable Payable, amount int, idempotencyKey string) (*PaymentResponse, error) {

	body := strings.NewReader(fmt.Sprintf(`{
		"amount": %d
	}`, amount))

	return makeRequestWithIdempotencyKey("POST", fmt.Sprintf("%s/v1/services/%s/payments/refund-partial-amount", apiUrl, payable.GetId()), body, idempotencyKey)
}

func CreateServiceWithMetadata(app core.App, payable Payable, payee Payee) error {
	request := map[string]interface{}{
		"organization_id": payee.GetOrganizationId(),
		"user_id":         payable.GetUserId(),
		"metadata":        payable.GetMetadata(app),
		"amount":          payable.GetAmount(),
		"service_id":      payable.GetId(),
	}
	requestJson, _ := json.Marshal(request)
	body := strings.NewReader(string(requestJson))

	_, err := makeRequest("POST", apiUrl+"/v1/services/create", body)

	return err
}

func UpdateService(app core.App, payable Payable, amount int) error {

	request := map[string]interface{}{
		"amount":   amount,
		"metadata": payable.GetMetadata(app),
	}
	requestJson, _ := json.Marshal(request)
	body := strings.NewReader(string(requestJson))

	_, err := makeRequest("PATCH", fmt.Sprintf("%s/v1/services/%s/update", apiUrl, payable.GetId()), body)

	return err
}

func CreatePayment(payable Payable, payee Payee, payment_type string) (*PaymentResponse, error) {
	return CreatePaymentWithIdempotencyKey(payable, payee, payment_type, "")
}

// CreatePaymentWithIdempotencyKey creates a payment and forwards the optional
// idempotency key to Payment API.
func CreatePaymentWithIdempotencyKey(payable Payable, payee Payee, paymentType string, idempotencyKey string) (*PaymentResponse, error) {

	body := strings.NewReader(fmt.Sprintf(`{
		"payment_type": "%s",
		"tpv_id": "%s"
	}`, paymentType, payee.GetTpvId()))

	r, err := makeRequestWithIdempotencyKey("POST", fmt.Sprintf("%s/v1/services/%s/payments/create", apiUrl, payable.GetId()), body, idempotencyKey)

	return r, err

}

func CreatePaymentByMethodId(payable Payable, payee Payee, payment_type string, paymentMethodId string) (*PaymentResponse, error) {
	return CreatePaymentByMethodIdWithIdempotencyKey(payable, payee, payment_type, paymentMethodId, "")
}

// CreatePaymentByMethodIdWithIdempotencyKey creates a payment with an explicit
// payment method and forwards the optional idempotency key.
func CreatePaymentByMethodIdWithIdempotencyKey(payable Payable, payee Payee, paymentType string, paymentMethodId string, idempotencyKey string) (*PaymentResponse, error) {

	body := strings.NewReader(fmt.Sprintf(`{
		"payment_type": "%s",
		"tpv_id": "%s",
		"payment_method_id": "%s"
	}`, paymentType, payee.GetTpvId(), paymentMethodId))

	r, err := makeRequestWithIdempotencyKey("POST", fmt.Sprintf("%s/v1/services/%s/payments/create", apiUrl, payable.GetId()), body, idempotencyKey)

	return r, err
}

func CreateRedirectPayment(payable Payable, payee Payee, returnUrlOk string, returnUrlKo string, returnUrlNotification string) (*RedirectPaymentResponse, error) {
	return CreateRedirectPaymentWithIdempotencyKey(payable, payee, returnUrlOk, returnUrlKo, returnUrlNotification, "")
}

// CreateRedirectPaymentWithIdempotencyKey creates a redirect payment and
// forwards the optional idempotency key.
func CreateRedirectPaymentWithIdempotencyKey(payable Payable, payee Payee, returnUrlOk string, returnUrlKo string, returnUrlNotification string, idempotencyKey string) (*RedirectPaymentResponse, error) {
	body := strings.NewReader(fmt.Sprintf(`{
		"url_ok": "%s",
		"url_ko": "%s",
		"url_notification": "%s",
		"tpv_id": "%s"
	}`, returnUrlOk, returnUrlKo, returnUrlNotification, payee.GetTpvId()))

	response, err := makeRedirectRequestWithIdempotencyKey("POST", fmt.Sprintf("%s/v1/services/%s/redirect-payments/create", apiUrl, payable.GetId()), body, idempotencyKey)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func ConfirmPreautorhization(payable Payable) error {
	_, err := ConfirmPreauthorizationWithIdempotencyKey(payable, "")
	return err
}

// ConfirmPreauthorizationWithIdempotencyKey confirms a preauthorization and
// returns the Payment API result, including an optional cashback completion.
func ConfirmPreauthorizationWithIdempotencyKey(payable Payable, idempotencyKey string) (*PaymentResponse, error) {

	body := strings.NewReader(`{}`)

	return makeRequestWithIdempotencyKey("POST", fmt.Sprintf("%s/v1/services/%s/payments/confirm", apiUrl, payable.GetId()), body, idempotencyKey)
}

func CancelPreautorhization(payable Payable) error {
	_, err := CancelPreauthorizationWithIdempotencyKey(payable, "")
	return err
}

// CancelPreauthorizationWithIdempotencyKey cancels a preauthorization and
// returns the Payment API result.
func CancelPreauthorizationWithIdempotencyKey(payable Payable, idempotencyKey string) (*PaymentResponse, error) {

	body := strings.NewReader(`{}`)

	return makeRequestWithIdempotencyKey("POST", fmt.Sprintf("%s/v1/services/%s/payments/cancel", apiUrl, payable.GetId()), body, idempotencyKey)
}

func RefundPayment(payable Payable) error {
	_, err := RefundPaymentWithIdempotencyKey(payable, "")
	return err
}

// RefundPaymentWithIdempotencyKey refunds a service and returns the Payment API
// result, including cashback reversal fields when present.
func RefundPaymentWithIdempotencyKey(payable Payable, idempotencyKey string) (*PaymentResponse, error) {
	body := strings.NewReader(`{}`)

	return makeRequestWithIdempotencyKey("POST", fmt.Sprintf("%s/v1/services/%s/payments/refund", apiUrl, payable.GetId()), body, idempotencyKey)
}

func makeRequest(method string, url string, body *strings.Reader) (*PaymentResponse, error) {
	return makeRequestWithIdempotencyKey(method, url, body, "")
}

func makeRequestWithIdempotencyKey(method string, url string, body *strings.Reader, idempotencyKey string) (*PaymentResponse, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", apiToken)
	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}

	client := &http.Client{}
	response, err := client.Do(req)

	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		return nil, fmt.Errorf("api-payment-error: %d", response.StatusCode)
	}

	paymentResponse := &PaymentResponse{}
	err = json.NewDecoder(response.Body).Decode(paymentResponse)
	if err != nil {
		return nil, fmt.Errorf("api-payment-error: %s", err.Error())
	}

	return paymentResponse, nil
}

func makeRedirectRequest(method string, url string, body *strings.Reader) (*RedirectPaymentResponse, error) {
	return makeRedirectRequestWithIdempotencyKey(method, url, body, "")
}

func makeRedirectRequestWithIdempotencyKey(method string, url string, body *strings.Reader, idempotencyKey string) (*RedirectPaymentResponse, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", apiToken)
	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}

	client := &http.Client{}
	response, err := client.Do(req)

	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		return nil, fmt.Errorf("api-payment-error: %d", response.StatusCode)
	}

	paymentResponse := &RedirectPaymentResponse{}
	err = json.NewDecoder(response.Body).Decode(paymentResponse)
	if err != nil {
		return nil, fmt.Errorf("api-payment-error: %s", err.Error())
	}

	return paymentResponse, nil
}

type PaymentResponse struct {
	Payable   PayableResponse `json:"Payable"`
	PaymentId string          `json:"payment_id,omitempty"`
	CashbackResponse
}
type PayableResponse struct {
	Id            string `json:"id"`
	LastPaymentId string `json:"last_payment_id"`
}

func (p PaymentResponse) GetPaymentId() string {
	if p.PaymentId != "" {
		return p.PaymentId
	}
	return p.Payable.LastPaymentId
}

// CashbackResponse contains the optional fields returned when Payment API
// evaluates cashback. Zero values mean that the legacy response omitted them.
type CashbackResponse struct {
	ServiceId                    string `json:"service_id,omitempty"`
	CompletionType               string `json:"completion_type,omitempty"`
	OriginalAmount               int    `json:"original_amount,omitempty"`
	AmountCharged                int    `json:"amount_charged,omitempty"`
	CashbackDiscountAmount       int    `json:"cashback_discount_amount,omitempty"`
	CashbackRedeemedPoints       int    `json:"cashback_redeemed_points,omitempty"`
	CashbackEarnedPoints         int    `json:"cashback_earned_points,omitempty"`
	CashbackRedeemRestoredPoints int    `json:"cashback_redeem_restored_points,omitempty"`
	CashbackEarnReversedPoints   int    `json:"cashback_earn_reversed_points,omitempty"`
	CashbackFullyReversed        bool   `json:"cashback_fully_reversed,omitempty"`
	CashbackRefundStatus         string `json:"cashback_refund_status,omitempty"`
	CashbackStatus               string `json:"cashback_status,omitempty"`
}

type RedirectPaymentResponse struct {
	PaymentId            string `json:"payment_id"`
	OrderId              string `json:"order_id,omitempty"`
	DsMerchantParameters string `json:"ds_merchant_parameters"`
	DsSignatureVersion   string `json:"ds_signature_version"`
	DsSignature          string `json:"ds_signature"`
	RedsysUrl            string `json:"redsys_url"`
	CashbackResponse
}
