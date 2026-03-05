package innpark

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/apache/arrow/go/v15/arrow/array"
	"github.com/pocketbase/pocketbase/core"
)

var onstreetUrl = os.Getenv("API_ONSTREET_URL")
var onstreetToken = os.Getenv("API_ONSTREET_TOKEN")

func GetPlateLists(plate string, startDateTime string) []ListItem {
	url := fmt.Sprintf(
		"%s/v1/lists/get-plate-lists?plate=%s&startDateTime=%s", onstreetUrl, plate, startDateTime)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return []ListItem{}
	}
	req.Header.Set("Authorization", onstreetToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	response, err := client.Do(req)

	if err != nil {
		return []ListItem{}
	}

	if response.StatusCode != 200 {
		responseBody := make([]byte, response.ContentLength)
		response.Body.Read(responseBody)
		return []ListItem{}
	}

	lists := []ListItem{}
	err = json.NewDecoder(response.Body).Decode(&lists)
	if err != nil {
		return []ListItem{}
	}

	return lists

}

func GetEnrichedPlateLists(plate string, startDateTime string) []EnrichedListItem {
	url := fmt.Sprintf(
		"%s/v1/lists/get-enriched-plate-lists?plate=%s&startDateTime=%s", onstreetUrl, plate, startDateTime)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return []EnrichedListItem{}
	}
	req.Header.Set("Authorization", onstreetToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	response, err := client.Do(req)

	if err != nil {
		return []EnrichedListItem{}
	}

	if response.StatusCode != 200 {
		responseBody := make([]byte, response.ContentLength)
		response.Body.Read(responseBody)
		return []EnrichedListItem{}
	}

	lists := []EnrichedListItem{}
	err = json.NewDecoder(response.Body).Decode(&lists)
	if err != nil {
		return []EnrichedListItem{}
	}

	return lists

}

func GetPlatesInList(
	app core.App,
	listId string) []string {

	var plates []string
	page := 1
	perPage := 500 // Máximo permitido por pocketbase

	for {
		url := fmt.Sprintf(
			"%s/collections/list_items/records?filter=(list_id='%s')&page=%d&perPage=%d&fields=value", onstreetUrl, listId, page, perPage)

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
			Items      []Plates `json:"items"`
			Page       int      `json:"page"`
			PerPage    int      `json:"perPage"`
			TotalItems int      `json:"totalItems"`
			TotalPages int      `json:"totalPages"`
		}{}
		err = json.NewDecoder(response.Body).Decode(listItemsResponse)
		if err != nil {
			app.Logger().Error("error decoding response in list", "error", err)
			return []string{}
		}

		for _, item := range listItemsResponse.Items {
			plates = append(plates, item.Value)
		}

		if page >= listItemsResponse.TotalPages {
			break
		}
		page++
	}

	return plates
}

// type ListItem_OLD struct {
// 	Id string `json:"list_id"`
// }

type ListItem struct {
	ListId   string `json:"list_id"`
	FromDate string `json:"from_date"`
	ToDate   string `json:"to_date"`
}

type FreeBag struct {
	Seconds      int         `json:"seconds"`
	Segments     int         `json:"segments"`
	UsableInDays array.Int64 `json:"usable_in_days"`
}

type EnrichedListItem struct {
	ListItem
	FreeBag
}

type Plates struct {
	Value string `json:"value"`
}
