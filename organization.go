package innpark

import (
	"github.com/pocketbase/pocketbase/tools/types"
)

func BuildOrganizationMap(id string, info types.JsonMap) map[string]interface{} {
	return map[string]interface{}{
		"id":            id,
		"name":          info.Get("name"),
		"cif":           info.Get("cif"),
		"address_1":     info.Get("address_1"),
		"address_2":     info.Get("address_2"),
		"city":          info.Get("city"),
		"country":       info.Get("country"),
		"postal_code":   info.Get("postal_code"),
		"phone":         info.Get("phone"),
		"email":         info.Get("email"),
		"taxe":          info.Get("taxe"),
		"logo":          info.Get("logo"),
		"website":       info.Get("website"),
		"primary_color": info.Get("primary_color"),
	}
}
