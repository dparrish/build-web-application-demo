package authentication

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/dparrish/build-web-application-demo/autoconfig"
	"github.com/dparrish/build-web-application-demo/swagger"

	auth0 "github.com/auth0-community/go-auth0"
	gctx "github.com/gorilla/context"
	jose "gopkg.in/square/go-jose.v2"
	"gopkg.in/square/go-jose.v2/jwt"
)

func Handler(config *autoconfig.Config) http.HandlerFunc {
	if config.Get("auth0.domain") == "" || config.Get("auth0.audience") == "" || config.Get("auth0.client_id") == "" || config.Get("auth0.client_secret") == "" {
		log.Fatalf("Unable to create authentication handler, auth0 is not configured")
	}
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the login request from the client.
		var user map[string]string
		if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
			swagger.Errorf(w, http.StatusForbidden, "Invalid login request")
			return
		}

		// Pass the user authentication information to Auth0 for validation.
		payload, _ := json.Marshal(map[string]string{
			"grant_type":    "http://auth0.com/oauth/grant-type/password-realm",
			"username":      user["email"],
			"password":      user["password"],
			"realm":         "Username-Password-Authentication",
			"scope":         "openid profile email",
			"audience":      config.Get("auth0.audience"),
			"client_id":     config.Get("auth0.client_id"),
			"client_secret": config.Get("auth0.client_secret"),
		})
		req, err := http.NewRequest("POST", fmt.Sprintf("https://%s/oauth/token", config.Get("auth0.domain")), bytes.NewReader(payload))
		if err != nil {
			swagger.Errorf(w, http.StatusInternalServerError, "Error requesting OAuth token")
			return
		}
		req.Header.Add("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			swagger.Errorf(w, http.StatusInternalServerError, "Error requesting OAuth token")
			return
		}
		defer res.Body.Close()

		var authResponse map[string]string
		json.NewDecoder(res.Body).Decode(&authResponse)
		if authResponse["error"] != "" || authResponse["id_token"] == "" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(authResponse)
			return
		}

		json.NewEncoder(w).Encode(map[string]string{"token": authResponse["id_token"]})
	}
}

func Middleware(config *autoconfig.Config, next http.HandlerFunc) http.HandlerFunc {
	uri := fmt.Sprintf("https://%s/.well-known/jwks.json", config.Get("auth0.domain"))
	audience := []string{config.Get("auth0.client_id")}
	issuer := fmt.Sprintf("https://%s/", config.Get("auth0.domain"))
	client := auth0.NewJWKClient(auth0.JWKClientOptions{URI: uri})
	validator := auth0.NewValidator(auth0.NewConfiguration(client, audience, issuer, jose.RS256))

	return func(w http.ResponseWriter, r *http.Request) {
		token, err := validator.ValidateRequest(r)
		if err != nil {
			swagger.Errorf(w, http.StatusUnauthorized, "Missing or invalid token")
			return
		}

		var claims jwt.Claims
		validator.Claims(r, token, &claims)

		// Check the expiry time on the JWT claim.
		if time.Now().After(claims.Expiry.Time()) {
			swagger.Errorf(w, http.StatusUnauthorized, "Missing or invalid token")
			return
		}

		// Save the logged-in user ID to the context for the next handler.
		gctx.Set(r, "userid", claims.Subject)
		next.ServeHTTP(w, r)
	}
}
