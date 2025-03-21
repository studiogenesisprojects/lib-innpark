package innpark

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/pocketbase/pocketbase/core"
)

var onstreetUrl = os.Getenv("API_ONSTREET_URL")
var onstreetToken = os.Getenv("API_ONSTREET_TOKEN")

func GetPlateLists(
	plate string) []string {

	url := fmt.Sprintf(
		"%s/v1/lists/get-plate-lists?plate=%s", onstreetUrl, plate)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return []string{}
	}
	req.Header.Set("Authorization", onstreetToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	response, err := client.Do(req)

	if err != nil {
		return []string{}
	}

	if response.StatusCode != 200 {
		responseBody := make([]byte, response.ContentLength)
		response.Body.Read(responseBody)
		return []string{}
	}

	listsResponse := &[]ListItem{}
	err = json.NewDecoder(response.Body).Decode(listsResponse)
	if err != nil {
		return []string{}
	}

	var lists []string
	for _, item := range *listsResponse {
		lists = append(lists, item.Id)
	}

	return lists

}

func GetPlatesInList(
	app core.App,
	listId string) []string {

	url := fmt.Sprintf(
		"%s/collections/list_items/records?filter=(list_id='%s')&perPage=100000&fields=value", onstreetUrl, listId) //Hardcoded perPage

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		app.Logger().Error("error creating request", "error", err)
		return []string{}
	}
	req.Header.Set("Authorization", onstreetToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	response, err := client.Do(req)

	if err != nil {
		app.Logger().Error("error getting plates in list", "error", err)
		return []string{}
	}

	if response.StatusCode != 200 {
		responseBody := make([]byte, response.ContentLength)
		response.Body.Read(responseBody)

		app.Logger().Error("error getting plates in list",
			"url", url,
			"request", req,
			"response", response,
			"list_id", listId,
			"body", string(responseBody),
			"status", response.StatusCode)
		return []string{}
	}

	listItemsResponse := &struct {
		Items []Plates `json:"items"`
	}{}
	err = json.NewDecoder(response.Body).Decode(listItemsResponse)
	if err != nil {
		app.Logger().Error("error decoding response in list", "error", err)
		return []string{}
	}

	var plates []string
	for _, item := range listItemsResponse.Items {
		plates = append(plates, item.Value)
	}

	return plates
}

type ListItem struct {
	Id string `json:"list_id"`
}

type Plates struct {
	Value string `json:"value"`
}
