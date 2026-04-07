package image

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/docker/docker-credential-helpers/credentials"
)

var acrRE = regexp.MustCompile(`^.+\.azurecr\.(io|cn|de|us)$`)

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

	ctx, cancel := context.WithTimeout(context.Background(), acrTimeout)
	defer cancel()

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create azure credential: %w", err)
	}

	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to acquire access token: %w", err)
	}

	refreshToken, err := exchangeForACRRefreshToken(ctx, serverURL, token.Token)
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

func exchangeForACRRefreshToken(
	ctx context.Context,
	serverURL string,
	accessToken string,
) (string, error) {
	registryURL := "https://" + serverURL
	exchangeURL := registryURL + "/oauth2/exchange"

	form := url.Values{
		"grant_type":   {"access_token"},
		"service":      {serverURL},
		"access_token": {accessToken},
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
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf(
			"token exchange returned status %d: %s",
			resp.StatusCode,
			string(body),
		)
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
