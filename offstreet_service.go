package innpark

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

var offstreetUrl = os.Getenv("API_OFFSTREET_URL")

func CreateVehicle(plate string, userId string) (string, error) {
	// Create vehicle
	url := fmt.Sprintf("%s/v1/vehicles/create", offstreetUrl)

	body := strings.NewReader(fmt.Sprintf(`{	
		"plate": "%s",
		"user_id": "%s"
	}`, plate, userId))

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	response, err := client.Do(req)

	if err != nil {
		return "", err
	}

	if response.StatusCode != 200 {
		return "", fmt.Errorf("api-offstreet-error: %d", response.StatusCode)
	}

	createVehicleResponse := &CreateVehicleResponse{}
	err = json.NewDecoder(response.Body).Decode(createVehicleResponse)
	if err != nil {
		return "", fmt.Errorf("api-offstreet-error: %s", err.Error())
	}

	return createVehicleResponse.Id, nil
}

func DeleteVehicle(plate string, userId string) error {
	// delete vehicle
	url := fmt.Sprintf("%s/v1/vehicles/delete", offstreetUrl)

	body := strings.NewReader(fmt.Sprintf(`{	
		"plate": "%s",
		"user_id": "%s"
	}`, plate, userId))

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	response, err := client.Do(req)

	if err != nil {
		return err
	}

	if response.StatusCode != 200 {
		return fmt.Errorf("api-offstreet-error: %d", response.StatusCode)
	}

	return nil
}

type VehicleResponse struct {
	Id     string `json:"id"`
	UserId string `json:"user_id"`
	Plate  string `json:"plate"`
}

type CreateVehicleResponse struct {
	Id string `json:"id"`
}
