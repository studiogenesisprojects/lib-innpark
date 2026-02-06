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

func CreatePayment(payable Payable, payee Payee, payment_type string) error {

	body := strings.NewReader(fmt.Sprintf(`{
		"payment_type": "%s",
		"tpv_id": "%s"
	}`, payment_type, payee.GetTpvId()))

	_, err := makeRequest("POST", fmt.Sprintf("%s/v1/services/%s/payments/create", apiUrl, payable.GetId()), body)

	return err

}

func CreatePaymentByMethodId(payable Payable, payee Payee, payment_type string, paymentMethodId string) error {

	body := strings.NewReader(fmt.Sprintf(`{
		"payment_type": "%s",
		"tpv_id": "%s",
		"payment_method_id": "%s"
	}`, payment_type, payee.GetTpvId(), paymentMethodId))

	_, err := makeRequest("POST", fmt.Sprintf("%s/v1/services/%s/payments/create", apiUrl, payable.GetId()), body)

	return err
}

func CreateRedirectPayment(payable Payable, payee Payee, returnUrlOk string, returnUrlKo string, returnUrlNotification string) (*RedirectPaymentResponse, error) {
	body := strings.NewReader(fmt.Sprintf(`{
		"url_ok": "%s",
		"url_ko": "%s",
		"url_notification": "%s",
		"tpv_id": "%s"
	}`, returnUrlOk, returnUrlKo, returnUrlNotification, payee.GetTpvId()))

	response, err := makeRedirectRequest("POST", fmt.Sprintf("%s/v1/services/%s/redirect-payments/create", apiUrl, payable.GetId()), body)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func ConfirmPreautorhization(payable Payable) error {

	body := strings.NewReader(`{}`)

	_, err := makeRequest("POST", fmt.Sprintf("%s/v1/services/%s/payments/confirm", apiUrl, payable.GetId()), body)
	return err
}

func CancelPreautorhization(payable Payable) error {

	body := strings.NewReader(`{}`)

	_, err := makeRequest("POST", fmt.Sprintf("%s/v1/services/%s/payments/cancel", apiUrl, payable.GetId()), body)
	return err
}

func RefundPayment(payable Payable) error {
	body := strings.NewReader(`{}`)

	_, err := makeRequest("POST", fmt.Sprintf("%s/v1/services/%s/payments/refund", apiUrl, payable.GetId()), body)
	return err
}

func makeRequest(method string, url string, body *strings.Reader) (*PaymentResponse, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", apiToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	response, err := client.Do(req)

	if err != nil {
		return nil, err
	}

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
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", apiToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	response, err := client.Do(req)

	if err != nil {
		return nil, err
	}

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
	PaymentId string `json:"payment_id"`
}

func (p PaymentResponse) GetPaymentId() string {
	return p.PaymentId
}

type RedirectPaymentResponse struct {
	PaymentId            string `json:"payment_id"`
	DsMerchantParameters string `json:"ds_merchant_parameters"`
	DsSignatureVersion   string `json:"ds_signature_version"`
	DsSignature          string `json:"ds_signature"`
	RedsysUrl            string `json:"redsys_url"`
}
