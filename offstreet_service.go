package innpark

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

var offstreetUrl = os.Getenv("API_OFFSTREET_URL")

func CreateVehicle(plate string, vehicleId string, userId string) (string, error) {
	// Create vehicle
	url := fmt.Sprintf("%s/v1/vehicles/create", offstreetUrl)

	body := strings.NewReader(fmt.Sprintf(`{	
		"plate": "%s",
		"vehicle_id": "%s",
		"user_id": "%s"
	}`, plate, vehicleId, userId))

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

func GetParkings(organizationId string, clusterId string) []Parking {
	url := fmt.Sprintf("%s/collections/parkings/records", offstreetUrl)

	var filters []string
	if organizationId != "" {
		filters = append(filters, fmt.Sprintf("organization_id='%s'", organizationId))
	}
	if clusterId != "" {
		filters = append(filters, fmt.Sprintf("cluster_id='%s'", clusterId))
	}
	if len(filters) > 0 {
		url += fmt.Sprintf("?filter=(%s)", strings.Join(filters, " && "))
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return []Parking{}
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	response, err := client.Do(req)

	if err != nil {
		return []Parking{}
	}

	if response.StatusCode != 200 {
		return []Parking{}
	}

	var parkingsResponse ParkingResponse
	err = json.NewDecoder(response.Body).Decode(&parkingsResponse)
	if err != nil {
		return []Parking{}
	}

	return parkingsResponse.Items
}

type ParkingResponse struct {
	Items []Parking `json:"items"`
}

type VehicleResponse struct {
	Id     string `json:"id"`
	UserId string `json:"user_id"`
	Plate  string `json:"plate"`
}

type CreateVehicleResponse struct {
	Id string `json:"id"`
}
type Parking struct {
	Id             string `json:"id"`
	OrganizationId string `json:"organization_id"`
	ClusterId      string `json:"cluster_id"`
	Name           string `json:"name"`
}
