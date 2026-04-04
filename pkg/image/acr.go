package image

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/docker/docker-credential-helpers/credentials"
)

var acrRE = regexp.MustCompile(`.*\.azurecr\.io|.*\.azurecr\.cn|.*\.azurecr\.de|.*\.azurecr\.us`)

const (
	mcrHostname   = "mcr.microsoft.com"
	tokenUsername = "<token>"
	acrTimeout    = 30 * time.Second
)

type acrCredHelper struct{}

func newACRCredentialsHelper() credentials.Helper {
	return &acrCredHelper{}
}

func (a *acrCredHelper) Add(_ *credentials.Credentials) error {
	return errors.New("add is unimplemented")
}

func (a *acrCredHelper) Delete(_ string) error {
	return errors.New("delete is unimplemented")
}

func (a *acrCredHelper) List() (map[string]string, error) {
	return nil, errors.New("list is unimplemented")
}

func (a *acrCredHelper) Get(serverURL string) (string, string, error) {
	if !isACRRegistry(serverURL) {
		return "", "", errors.New("serverURL does not refer to Azure Container Registry")
	}

	spToken, settings, err := getServicePrincipalToken()
	if err != nil {
		return "", "", fmt.Errorf("failed to acquire sp token: %w", err)
	}

	refreshToken, err := exchangeForACRRefreshToken(
		serverURL, spToken, settings.Values[auth.TenantID],
	)
	if err != nil {
		return "", "", fmt.Errorf("failed to acquire refresh token: %w", err)
	}

	return tokenUsername, refreshToken, nil
}

func isACRRegistry(input string) bool {
	serverURL, err := url.Parse("https://" + input)
	if err != nil {
		return false
	}
	if serverURL.Hostname() == mcrHostname {
		return true
	}
	return acrRE.MatchString(serverURL.Hostname())
}

func getServicePrincipalToken() (
	*adal.ServicePrincipalToken, auth.EnvironmentSettings, error,
) {
	settings, err := auth.GetSettingsFromEnvironment()
	if err != nil {
		return nil, auth.EnvironmentSettings{}, fmt.Errorf(
			"failed to get auth settings from environment: %w", err,
		)
	}

	spToken, err := newServicePrincipalToken(
		settings, settings.Environment.ResourceManagerEndpoint,
	)
	if err != nil {
		return nil, auth.EnvironmentSettings{}, fmt.Errorf(
			"failed to initialise sp token config: %w", err,
		)
	}

	return spToken, settings, nil
}

func newServicePrincipalToken(
	settings auth.EnvironmentSettings, resource string,
) (*adal.ServicePrincipalToken, error) {
	// 1. Client Credentials
	if cc, err := settings.GetClientCredentials(); err == nil {
		oAuthConfig, oauthErr := adal.NewOAuthConfig(
			settings.Environment.ActiveDirectoryEndpoint, cc.TenantID,
		)
		if oauthErr != nil {
			return nil, fmt.Errorf("failed to initialise OAuthConfig: %w", oauthErr)
		}
		return adal.NewServicePrincipalToken(
			*oAuthConfig, cc.ClientID, cc.ClientSecret, cc.Resource,
		)
	}

	// 2. Federated OIDC JWT assertion
	if jwt, jwtErr := lookupFederatedJWT(); jwtErr == nil {
		clientID := os.Getenv("AZURE_CLIENT_ID")
		if clientID == "" {
			return nil, fmt.Errorf("AZURE_CLIENT_ID not set")
		}
		tenantID := os.Getenv("AZURE_TENANT_ID")
		if tenantID == "" {
			return nil, fmt.Errorf("AZURE_TENANT_ID not set")
		}
		oAuthConfig, oauthErr := adal.NewOAuthConfig(
			settings.Environment.ActiveDirectoryEndpoint, tenantID,
		)
		if oauthErr != nil {
			return nil, fmt.Errorf("failed to initialise OAuthConfig: %w", oauthErr)
		}
		return adal.NewServicePrincipalTokenFromFederatedTokenCallback(
			*oAuthConfig, clientID,
			func() (string, error) { return jwt, nil },
			resource,
		)
	}

	// 3. MSI
	return adal.NewServicePrincipalTokenFromManagedIdentity(
		resource, &adal.ManagedIdentityOptions{
			ClientID: os.Getenv("AZURE_CLIENT_ID"),
		},
	)
}

func lookupFederatedJWT() (string, error) {
	if jwt, ok := os.LookupEnv("AZURE_FEDERATED_TOKEN"); ok {
		return jwt, nil
	}
	if jwtFile, ok := os.LookupEnv("AZURE_FEDERATED_TOKEN_FILE"); ok {
		b, err := os.ReadFile(jwtFile) //nolint:gosec // path from trusted env var
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	return "", fmt.Errorf("no federated JWT found")
}

func exchangeForACRRefreshToken(
	serverURL string,
	principalToken *adal.ServicePrincipalToken,
	tenantID string,
) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), acrTimeout)
	defer cancel()

	principalToken.MaxMSIRefreshAttempts = 1
	if err := principalToken.EnsureFreshWithContext(ctx); err != nil {
		return "", fmt.Errorf("error refreshing sp token: %w", err)
	}

	registryURL := "https://" + serverURL
	exchangeURL := registryURL + "/oauth2/exchange"

	form := url.Values{
		"grant_type":   {"access_token"},
		"service":      {serverURL},
		"tenant":       {tenantID},
		"access_token": {principalToken.Token().AccessToken},
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, exchangeURL, strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to exchange token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token exchange returned status %d", resp.StatusCode)
	}

	var result struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode exchange response: %w", err)
	}

	if result.RefreshToken == "" {
		return "", fmt.Errorf("exchange returned empty refresh token")
	}

	return result.RefreshToken, nil
}
