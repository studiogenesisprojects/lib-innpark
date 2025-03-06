package innpark

import (
	"context"
	"os"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"github.com/pocketbase/pocketbase/core"
	"google.golang.org/api/option"

	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/models"
)

const (
	USERS_TENENT = "INNPARK-USERS-9cfl7"
	ADMIN_TENENT = "INNPARK-ADMINS-07uhs"
)

func Auth(app core.App, target string) echo.HandlerFunc {
	return func(c echo.Context) error {

		tenantToUse := ""
		switch target {
		case "users":
			tenantToUse = USERS_TENENT
		case "admins":
			tenantToUse = ADMIN_TENENT
		default:
			return apis.NewUnauthorizedError("invalid tenant", nil)
		}

		idToken := c.QueryParam("token")
		if idToken == "" {
			return apis.NewUnauthorizedError("missing token", nil)
		}

		token, err := veifyFirebaseToken(idToken)
		if err != nil {
			return apis.NewUnauthorizedError("invalid token", nil)
		}

		if token.Firebase.Tenant != tenantToUse {
			return apis.NewUnauthorizedError("invalid tenant", nil)
		}

		userId := token.UID

		//check if user exists in db
		user, err := app.Dao().FindRecordById(target, userId)

		if err != nil {
			userInFirebase, err := getFirebaseUser(token.UID, tenantToUse)
			if err != nil {
				return apis.NewUnauthorizedError("user-not-found", err)
			}

			//create user
			collection, err := app.Dao().FindCollectionByNameOrId("users")
			if err != nil {
				return err
			}

			userProviderInfo, err := getProviderUserInf(userInFirebase.ProviderUserInfo, token.Firebase.SignInProvider)

			if err != nil {
				return err
			}

			user = models.NewRecord(collection)
			user.SetId(userInFirebase.UID)
			user.Set("email", userProviderInfo.Email)
			user.Set("username", userInFirebase.UID)
			user.Set("name", userProviderInfo.DisplayName)
			user.Set("photo_url", userProviderInfo.PhotoURL)
			user.Set("tokenKey", userInFirebase.UID)
			user.Set("password", userInFirebase.UID)
			user.Set("emailVisibility", true)
			if err := app.Dao().SaveRecord(user); err != nil {
				return apis.NewApiError(500, "failed to create user", err)
			}
		}

		return apis.RecordAuthResponse(app, c, user, nil)

	}
}

func veifyFirebaseToken(token string) (*auth.Token, error) {
	if client, err := getClient(); err != nil {
		return nil, err
	} else {
		return client.VerifyIDToken(context.Background(), token)
	}
}

func getFirebaseUser(uid string, tentantId string) (*auth.UserRecord, error) {

	if client, err := getClient(); err != nil {
		return nil, err
	} else {
		client.TenantManager.AuthForTenant(tentantId)
		return client.GetUser(context.Background(), uid)
	}
}

func getClient() (*auth.Client, error) {
	if currentDir, err := os.Getwd(); err != nil {
		return nil, err
	} else {
		opt := option.WithCredentialsFile(currentDir + "/serviceAccountKey.json")
		firebaseConfig := &firebase.Config{
			ProjectID: "innpark",
		}
		if fb, err := firebase.NewApp(context.Background(), firebaseConfig, opt); err != nil {
			return nil, err
		} else {
			return fb.Auth(context.Background())
		}
	}
}

func getProviderUserInf(users []*auth.UserInfo, provider string) (*auth.UserInfo, error) {
	for _, user := range users {
		if user.ProviderID == provider {
			return user, nil
		}
	}
	return nil, apis.NewUnauthorizedError("provider-not-found", nil)

}
