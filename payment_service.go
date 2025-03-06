package innpark

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

type PayableMetadata struct {
	Type          string `json:"type"`
	LocationType  string `json:"location_type"`
	LocationId    string `json:"location_id"`
	LocationName  string `json:"location_name"`
	VehicleId     string `json:"vehicle_id"`
	VehiclePlate  string `json:"vehicle_plate"`
	VehicleName   string `json:"vehicle_name"`
	StartDateTime string `json:"start_date_time"`
	EndDateTime   string `json:"end_date_time"`
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

func RefundPayment(payment Payment) error {
	body := strings.NewReader(fmt.Sprintf(`{
		"payment_id": "%s"
	}`, payment.GetApiPaymentId()))

	_, err := makeRequest("POST", apiUrl+"/v1/payments/refund", body)
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

type PaymentResponse struct {
	PaymentId string `json:"payment_id"`
}

func (p PaymentResponse) GetPaymentId() string {
	return p.PaymentId
}
