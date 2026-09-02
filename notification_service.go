package innpark

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	novu "github.com/novuhq/go-novu/lib"
)

type CredentialsRequest struct {
	IntegrationIdentifier string `json:"integrationIdentifier"`
	ProviderId            string `json:"providerId"`
	Credentials           struct {
		DeviceTokens []string `json:"deviceTokens"`
	} `json:"credentials"`
}

const (
	FIREBASE_CLOUD_MESSAGING = "firebase-cloud-messaging-PZsqbhqPe"
)

const (
	WORKFLOW_APARKPLUS_ACCEPTED = "aparkplus-accepted"
	WORKFLOW_APARKPLUS_REFUSED  = "aparkplus-refused"
	WORKFLOW_WELCOME            = "welcome-email"

	WORKFLOW_PAYMENT_ERROR                      = "payment-error"
	WORKFLOW_PAYMENT_SUCCESS                    = "payment-success"
	WORKFLOW_NEW_PAYMENT_METHOD                 = "new-payment-method-email"
	WORKFLOW_PAYMENT_METHOD_EXPIRATION_REMINDER = "payment-method-expiration-reminder"
	WORKFLOW_SERVICES_EMAIL                     = "services-email"

	// Onstreet
	WORKFLOW_ONSTREET_STAY_REMINDER  = "onstreet-stay-reminder"
	WORKFLOW_ONSTREET_STAY_COMPLETED = "onstreet-stay-completed"

	//Offstreet
	OFFSTREET_STAY_AFTER_EXIT  = "offstreet-stay-after-exit"
	OFFSTREET_STAY_AFTER_ENTER = "offstreet-stay-after-enter"

	WORKFLOW_LIST_ITEM_EXPIRATION_REMINDER = "list-item-expiration-reminder"

	WORKFLOW_NEW_COMPLAINT = "new-complaint"
)

func UpdateSubscriberCredentials(
	userId string,
	tokens []string,
) error {
	baseURL, err := novuAPIBaseURL()
	if err != nil {
		return err
	}

	endpoint := fmt.Sprintf("%s/v1/subscribers/%s/credentials", baseURL.String(), url.PathEscape(userId))

	request := CredentialsRequest{
		ProviderId:            "fcm",
		IntegrationIdentifier: os.Getenv("NOVU_FCM_ID"),
	}

	request.Credentials.DeviceTokens = append(request.Credentials.DeviceTokens, tokens...)

	j, err := json.Marshal(request)
	if err != nil {
		return err
	}
	payload := strings.NewReader(string(j))

	req, err := http.NewRequest("PUT", endpoint, payload)
	if err != nil {
		return err
	}

	req.Header.Add("Authorization", "ApiKey "+os.Getenv("NOVU_TOKEN"))
	req.Header.Add("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		errorBody, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("error updating subscriber credentials: %s", string(errorBody))
	}

	return nil
}

func novuAPIBaseURL() (*url.URL, error) {
	rawURL := strings.TrimSpace(os.Getenv("NOVU_API_URL"))
	if rawURL == "" {
		rawURL = "https://api.novu.co"
	}

	parsedURL, err := url.Parse(strings.TrimRight(rawURL, "/"))
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("invalid NOVU_API_URL")
	}
	parsedURL.Path = strings.TrimSuffix(strings.TrimRight(parsedURL.Path, "/"), "/v1")
	parsedURL.RawQuery = ""
	parsedURL.Fragment = ""
	return parsedURL, nil
}

func NewNovuClient() (*novu.APIClient, error) {
	baseURL, err := novuAPIBaseURL()
	if err != nil {
		return nil, err
	}
	return novu.NewAPIClient(os.Getenv("NOVU_TOKEN"), &novu.Config{BackendURL: baseURL}), nil
}

func CreateSubscriber(userID string, email string) error {
	novuClient, err := NewNovuClient()
	if err != nil {
		return err
	}
	_, err = novuClient.SubscriberApi.Identify(context.Background(), userID, map[string]interface{}{
		"subscriberId": userID,
		"email":        email,
		"locale":       "ca",
	})

	return err
}

func TriggerWorkflow(workflowName string, subscriberId string, payload map[string]interface{}) error {
	ctx := context.Background()
	novuClient, err := NewNovuClient()
	if err != nil {
		return err
	}

	payloadOptions := novu.ITriggerPayloadOptions{
		To: map[string]interface{}{
			"subscriberId": subscriberId,
		},
		Payload: payload,
	}
	_, err = novuClient.EventApi.Trigger(ctx, workflowName, payloadOptions)

	if err != nil {
		return err
	}

	return nil
}

// TriggerWorkflowForOrganization triggers a workflow via the payment API,
// which will restrict FCM delivery to tokens registered under the given organization.
func TriggerWorkflowForOrganization(workflowName string, userId string, organizationId string, payload map[string]interface{}) error {
	body := struct {
		WorkflowName   string                 `json:"workflow_name"`
		UserId         string                 `json:"user_id"`
		OrganizationId string                 `json:"organization_id"`
		Payload        map[string]interface{} `json:"payload"`
	}{
		WorkflowName:   workflowName,
		UserId:         userId,
		OrganizationId: organizationId,
		Payload:        payload,
	}

	j, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", os.Getenv("API_PAYMENT")+"/v1/notifications/trigger-for-organization", strings.NewReader(string(j)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", os.Getenv("API_PAYMENT_TOKEN"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("trigger-for-organization failed: %d", resp.StatusCode)
	}

	return nil
}

type Subscriber struct {
	ID           string `json:"_id"`
	SubscriberID string `json:"subscriberId"`
	Locale       string `json:"locale"`
	Email        string `json:"email"`
}

func GetSubscriber(userID string) (Subscriber, error) {
	novuClient, err := NewNovuClient()
	if err != nil {
		return Subscriber{}, err
	}
	resp, err := novuClient.SubscriberApi.Get(context.Background(), userID)
	if err != nil {
		return Subscriber{}, err
	}

	data, ok := resp.Data.(map[string]any)
	if !ok {
		return Subscriber{}, fmt.Errorf("unexpected subscriber payload")
	}

	return Subscriber{
		ID:           data["_id"].(string),
		SubscriberID: data["subscriberId"].(string),
		Locale:       data["locale"].(string),
		Email:        data["email"].(string),
	}, nil
}

func DeleteSubscriber(userID string) error {
	novuClient, err := NewNovuClient()
	if err != nil {
		return err
	}
	_, err = novuClient.SubscriberApi.Delete(context.Background(), userID)
	return err
}
