package pro

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	netUrl "net/url"
	"strings"

	"github.com/loft-sh/api/v4/pkg/auth"
	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/platform/client"
	"github.com/skratchdot/open-golang/open"
)

func (cmd *StartCmd) login(url string) error {
	if !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}

	// check if we are already logged in
	if cmd.isLoggedIn(url) {
		// still open the UI
		err := open.Run(url)
		if err != nil {
			return fmt.Errorf("couldn't open the login page in a browser: %w", err)
		}

		return nil
	}

	// log into the CLI
	err := cmd.loginViaCLI(url)
	if err != nil {
		return err
	}

	// log into the UI
	err = cmd.loginUI(url)
	if err != nil {
		return err
	}

	return nil
}

func (cmd *StartCmd) loginViaCLI(url string) error {
	loginPath := "%s/auth/password/login"

	loginRequest := auth.PasswordLoginRequest{
		Username: defaultUser,
		Password: cmd.Password,
	}
	loginRequestBytes, err := json.Marshal(loginRequest)
	if err != nil {
		return err
	}
	loginRequestBuf := bytes.NewBuffer(loginRequestBytes)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := &http.Client{Transport: tr}

	resp, err := httpClient.Post(fmt.Sprintf(loginPath, url), "application/json", loginRequestBuf)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	accessKey := &auth.AccessKey{}
	err = json.Unmarshal(body, accessKey)
	if err != nil {
		return err
	}

	// log into loft
	loader, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	url = strings.TrimSuffix(url, "/")
	err = loader.LoginWithAccessKey(url, accessKey.AccessKey, true, false)
	if err != nil {
		return err
	}

	cmd.Log.WriteString(logrus.InfoLevel, "\n")
	cmd.Log.WithFields(logrus.Fields{
		"url": url,
	}).Donef("logged in via CLI")

	return nil
}

func (cmd *StartCmd) loginUI(url string) error {
	queryString := fmt.Sprintf("username=%s&password=%s", defaultUser, netUrl.QueryEscape(cmd.Password))
	loginURL := fmt.Sprintf("%s/login#%s", url, queryString)

	err := open.Run(loginURL)
	if err != nil {
		return fmt.Errorf("couldn't open the login page in a browser: %w", err)
	}

	cmd.Log.Infof("If the browser does not open automatically, please navigate to %s", loginURL)

	return nil
}

func (cmd *StartCmd) isLoggedIn(url string) bool {
	url = strings.TrimPrefix(url, "https://")

	c, err := client.NewClientFromPath(cmd.Config)
	return err == nil && strings.TrimPrefix(strings.TrimSuffix(c.Config().Host, "/"), "https://") == strings.TrimSuffix(url, "/")
}
