package innpark

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	novu "github.com/novuhq/go-novu/lib"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
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
	WORKFLOW_APARKPLUS_ACCEPTED                 = "aparkplus-accepted"
	WORKFLOW_APARKPLUS_REFUSED                  = "aparkplus-refused"
	WORKFLOW_WELCOME                            = "welcome-email"
	WORKFLOW_STAY_REMINDER                      = "onstreet-stay-reminder"
	WORKFLOW_PAYMENT_ERROR                      = "payment-error"
	WORKFLOW_PAYMENT_SUCCESS                    = "payment-success"
	WORKFLOW_SERVICES_EMAIL                     = "services-email"
	WORKFLOW_NEW_PAYMENT_METHOD                 = "new-payment-method-email"
	WORKFLOW_PAYMENT_METHOD_EXPIRATION_REMINDER = "payment-method-expiration-reminder"
)

func UpdateSubscriberCredentials(
	app core.App,
	user *models.Record,
) error {

	url := fmt.Sprintf("https://api.novu.co/v1/subscribers/%s/credentials", user.Id)

	request := CredentialsRequest{
		ProviderId:            "fcm",
		IntegrationIdentifier: FIREBASE_CLOUD_MESSAGING,
	}

	userTokens, err := app.Dao().FindRecordsByFilter("fcm_devices", "user_id = {:user_id}", "created", 100, 0, dbx.Params{"user_id": user.Id})

	if err != nil {
		app.Logger().Error("fcm_devices_not_found", "err", err)
		return err
	}

	for _, token := range userTokens {
		request.Credentials.DeviceTokens = append(request.Credentials.DeviceTokens, token.GetString("token"))
	}

	j, err := json.Marshal(request)
	if err != nil {
		app.Logger().Error("error_marshal_json", "err", err)
		return err
	}
	payload := strings.NewReader(string(j))

	req, _ := http.NewRequest("PUT", url, payload)

	req.Header.Add("Authorization", "ApiKey "+os.Getenv("NOVU_TOKEN"))
	req.Header.Add("Content-Type", "application/json")

	res, _ := http.DefaultClient.Do(req)

	if res.StatusCode != 200 {
		errorBody := make([]byte, res.ContentLength)
		res.Body.Read(errorBody)
		return fmt.Errorf("error updating subscriber credentials: %s", string(errorBody))
	}

	defer res.Body.Close()

	return nil
}

func CreateSubscriber(userID string, email string) error {
	novuClient := novu.NewAPIClient(os.Getenv("NOVU_TOKEN"), &novu.Config{})
	_, err := novuClient.SubscriberApi.Identify(context.Background(), userID, map[string]interface{}{
		"subscriberId": userID,
		"email":        email,
		"locale":       "ca",
	})

	return err
}

func TriggerWorkflow(workflowName string, subscriberId string, payload map[string]interface{}) error {
	ctx := context.Background()
	novuClient := novu.NewAPIClient(os.Getenv("NOVU_TOKEN"), &novu.Config{})

	payloadOptions := novu.ITriggerPayloadOptions{
		To: map[string]interface{}{
			"subscriberId": subscriberId,
		},
		Payload: payload,
	}
	_, err := novuClient.EventApi.Trigger(ctx, workflowName, payloadOptions)

	if err != nil {
		return err
	}

	return nil
}
