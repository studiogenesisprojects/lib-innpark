package innpark

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

type paymentContractTestPayable struct{}

func (paymentContractTestPayable) GetId() string     { return "stay-1" }
func (paymentContractTestPayable) GetAmount() int    { return 1_000 }
func (paymentContractTestPayable) GetUserId() string { return "user-1" }
func (paymentContractTestPayable) GetMetadata(core.App) PayableMetadata {
	return PayableMetadata{Type: "ONSTREET", LocationId: "zone-1", VehiclePlate: "1234ABC"}
}

type paymentContractTestPayee struct{}

func (paymentContractTestPayee) GetTpvId() string          { return "tpv-1" }
func (paymentContractTestPayee) GetOrganizationId() string { return "organization-1" }

var (
	_ func(Payable, Payee, string) (*PaymentResponse, error)                         = CreatePayment
	_ func(Payable, Payee, string, string) (*PaymentResponse, error)                 = CreatePaymentByMethodId
	_ func(Payable) error                                                            = ConfirmPreautorhization
	_ func(Payable) error                                                            = CancelPreautorhization
	_ func(Payable) error                                                            = RefundPayment
	_ func(Payable, int) error                                                       = RefundPartialPaymentFromService
	_ func(Payable, Payee, string, string, string) (*RedirectPaymentResponse, error) = CreateRedirectPayment
)

func TestPaymentResponseSupportsLegacyAndCashbackContracts(t *testing.T) {
	legacyServer := paymentContractTestServer(t, func(response http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Idempotency-Key"); got != "" {
			t.Fatalf("legacy call unexpectedly sent idempotency key %q", got)
		}
		_, _ = response.Write([]byte(`{"Payable":{"id":"stay-1","last_payment_id":"payment-legacy"}}`))
	})
	legacy, err := CreatePayment(paymentContractTestPayable{}, paymentContractTestPayee{}, PAYMENT_TYPE_PAYMENT)
	legacyServer.Close()
	if err != nil {
		t.Fatal(err)
	}
	if legacy.GetPaymentId() != "payment-legacy" || legacy.CompletionType != "" {
		t.Fatalf("legacy response changed: %+v", legacy)
	}

	cashbackServer := paymentContractTestServer(t, func(response http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Idempotency-Key"); got != "onstreet:stay-1:payment:v1" {
			t.Fatalf("idempotency key=%q", got)
		}
		_, _ = response.Write([]byte(`{
			"payment_id":"payment-cashback",
			"service_id":"stay-1",
			"completion_type":"CASHBACK_PAYMENT",
			"original_amount":1000,
			"amount_charged":0,
			"cashback_discount_amount":1000,
			"cashback_redeemed_points":1000,
			"cashback_earned_points":0,
			"cashback_status":"SETTLED"
		}`))
	})
	cashback, err := CreatePaymentWithIdempotencyKey(
		paymentContractTestPayable{}, paymentContractTestPayee{}, PAYMENT_TYPE_PAYMENT, "onstreet:stay-1:payment:v1",
	)
	cashbackServer.Close()
	if err != nil {
		t.Fatal(err)
	}
	if cashback.GetPaymentId() != "payment-cashback" || cashback.CompletionType != "CASHBACK_PAYMENT" ||
		cashback.OriginalAmount != 1_000 || cashback.AmountCharged != 0 || cashback.CashbackDiscountAmount != 1_000 ||
		cashback.CashbackRedeemedPoints != 1_000 || cashback.CashbackStatus != "SETTLED" {
		t.Fatalf("cashback response was not decoded: %+v", cashback)
	}
}

func TestPaymentOperationsPropagateIdempotencyKey(t *testing.T) {
	tests := []struct {
		name string
		call func() error
	}{
		{name: "create", call: func() error {
			_, err := CreatePaymentWithIdempotencyKey(paymentContractTestPayable{}, paymentContractTestPayee{}, PAYMENT_TYPE_PAYMENT, "operation-key")
			return err
		}},
		{name: "create with method", call: func() error {
			_, err := CreatePaymentByMethodIdWithIdempotencyKey(paymentContractTestPayable{}, paymentContractTestPayee{}, PAYMENT_TYPE_PAYMENT, "method-1", "operation-key")
			return err
		}},
		{name: "confirm", call: func() error {
			_, err := ConfirmPreauthorizationWithIdempotencyKey(paymentContractTestPayable{}, "operation-key")
			return err
		}},
		{name: "cancel", call: func() error {
			_, err := CancelPreauthorizationWithIdempotencyKey(paymentContractTestPayable{}, "operation-key")
			return err
		}},
		{name: "refund", call: func() error {
			_, err := RefundPaymentWithIdempotencyKey(paymentContractTestPayable{}, "operation-key")
			return err
		}},
		{name: "partial refund", call: func() error {
			_, err := RefundPartialPaymentFromServiceWithIdempotencyKey(paymentContractTestPayable{}, 100, "operation-key")
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := paymentContractTestServer(t, func(response http.ResponseWriter, request *http.Request) {
				if got := request.Header.Get("Idempotency-Key"); got != "operation-key" {
					t.Fatalf("idempotency key=%q", got)
				}
				_, _ = response.Write([]byte(`{"payment_id":"payment-1"}`))
			})
			defer server.Close()
			if err := test.call(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRedirectPaymentPropagatesKeyAndDecodesCashback(t *testing.T) {
	server := paymentContractTestServer(t, func(response http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Idempotency-Key"); got != "redirect-key" {
			t.Fatalf("idempotency key=%q", got)
		}
		_, _ = response.Write([]byte(`{
			"order_id":"redirect-1",
			"completion_type":"CASHBACK_PAYMENT",
			"service_id":"stay-1",
			"original_amount":1000,
			"amount_charged":0,
			"cashback_discount_amount":1000,
			"cashback_redeemed_points":1000,
			"cashback_status":"SETTLED"
		}`))
	})
	defer server.Close()

	result, err := CreateRedirectPaymentWithIdempotencyKey(
		paymentContractTestPayable{}, paymentContractTestPayee{}, "https://ok", "https://ko", "https://callback", "redirect-key",
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.OrderId != "redirect-1" || result.CompletionType != "CASHBACK_PAYMENT" || result.CashbackRedeemedPoints != 1_000 {
		t.Fatalf("redirect cashback response was not decoded: %+v", result)
	}
}

func TestCreateServiceWithMetadataUsesExistingContract(t *testing.T) {
	server := paymentContractTestServer(t, func(response http.ResponseWriter, request *http.Request) {
		var payload struct {
			ServiceId      string          `json:"service_id"`
			OrganizationId string          `json:"organization_id"`
			Metadata       PayableMetadata `json:"metadata"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.ServiceId != "stay-1" || payload.OrganizationId != "organization-1" ||
			payload.Metadata.Type != "ONSTREET" || payload.Metadata.LocationId != "zone-1" || payload.Metadata.VehiclePlate != "1234ABC" {
			t.Fatalf("unexpected service payload: %+v", payload)
		}
		_, _ = response.Write([]byte(`{}`))
	})
	defer server.Close()

	if err := CreateServiceWithMetadata(nil, paymentContractTestPayable{}, paymentContractTestPayee{}); err != nil {
		t.Fatal(err)
	}
}

func paymentContractTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	previousURL := apiUrl
	previousToken := apiToken
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		if got := request.Header.Get("Authorization"); got != "test-token" {
			t.Fatalf("authorization=%q", got)
		}
		handler(response, request)
	}))
	apiUrl = server.URL
	apiToken = "test-token"
	t.Cleanup(func() {
		apiUrl = previousURL
		apiToken = previousToken
	})
	return server
}
