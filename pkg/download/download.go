package download

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/skevetter/devpod/pkg/gitcredentials"
	devpodhttp "github.com/skevetter/devpod/pkg/http"
	"github.com/skevetter/log"
)

// HTTPStatusError wraps HTTP status code errors for better error handling.
type HTTPStatusError struct {
	StatusCode int
	URL        string
	Body       string
}

func (e *HTTPStatusError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf(
			"received status code %d when trying to download %s: %s",
			e.StatusCode,
			e.URL,
			e.Body,
		)
	}
	return fmt.Sprintf(
		"received status code %d when trying to download %s",
		e.StatusCode,
		e.URL,
	)
}

func Head(rawURL string) (int, error) {
	req, err := http.NewRequest(http.MethodHead, rawURL, nil)
	if err != nil {
		return 0, err
	}

	resp, err := devpodhttp.GetHTTPClient().Do(req)
	if err != nil {
		return 0, fmt.Errorf("download file: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode, nil
}

func File(rawURL string, log log.Logger) (io.ReadCloser, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}

	if parsed.Host == "github.com" {
		// check if we can access the url
		code, err := Head(rawURL)
		if err != nil {
			return nil, err
		} else if code == 404 {
			// check if github release
			path := parsed.Path
			org, repo, release, file := parseGithubURL(path)
			if org != "" {
				// try to download with credentials if its a release
				log.Debugf("Try to find credentials for github")
				credentials, err := gitcredentials.GetCredentials(&gitcredentials.GitCredentials{
					Protocol: parsed.Scheme,
					Host:     parsed.Host,
					Path:     parsed.Path,
				})
				if err == nil && credentials != nil && credentials.Password != "" {
					log.Debugf("Make request with credentials")
					return downloadGithubRelease(org, repo, release, file, credentials.Password)
				}
			}
		}
	}

	resp, err := devpodhttp.GetHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("download file: %w", err)
	} else if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		_ = resp.Body.Close()
		return nil, &HTTPStatusError{StatusCode: resp.StatusCode, URL: rawURL, Body: string(body)}
	}

	return resp.Body, nil
}

type GithubRelease struct {
	Assets []GithubReleaseAsset `json:"assets,omitempty"`
}

type GithubReleaseAsset struct {
	ID   int    `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

func downloadGithubRelease(org, repo, release, file, token string) (io.ReadCloser, error) {
	var releasePath string
	if release == "" {
		releasePath = fmt.Sprintf(
			"/repos/%s/%s/releases/latest",
			url.PathEscape(org),
			url.PathEscape(repo),
		)
	} else {
		releasePath = fmt.Sprintf(
			"/repos/%s/%s/releases/tags/%s",
			url.PathEscape(org),
			url.PathEscape(repo),
			url.PathEscape(release),
		)
	}

	releaseURL := (&url.URL{
		Scheme: "https",
		Host:   "api.github.com",
		Path:   releasePath,
	}).String()

	req, err := http.NewRequest(http.MethodGet, releaseURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := devpodhttp.GetHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, &HTTPStatusError{
			StatusCode: resp.StatusCode,
			URL:        releaseURL,
			Body:       string(body),
		}
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	releaseObj := &GithubRelease{}
	err = json.Unmarshal(raw, releaseObj)
	if err != nil {
		return nil, err
	}

	var releaseAsset *GithubReleaseAsset
	for _, asset := range releaseObj.Assets {
		if asset.Name == file {
			releaseAsset = &asset
			break
		}
	}
	if releaseAsset == nil {
		return nil, fmt.Errorf("couldn't find asset %s in github release (%s)", file, releaseURL)
	}

	assetPath := fmt.Sprintf("/repos/%s/%s/releases/assets/%d", url.PathEscape(org), url.PathEscape(repo), releaseAsset.ID)
	assetURL := (&url.URL{
		Scheme: "https",
		Host:   "api.github.com",
		Path:   assetPath,
	}).String()

	req, err = http.NewRequest(http.MethodGet, assetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/octet-stream")
	downloadResp, err := devpodhttp.GetHTTPClient().Do(req)
	if err != nil {
		return nil, err
	} else if downloadResp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(downloadResp.Body, 1024))
		_ = downloadResp.Body.Close()
		return nil, &HTTPStatusError{
			StatusCode: downloadResp.StatusCode,
			URL:        assetURL,
			Body:       string(body),
		}
	}

	return downloadResp.Body, nil
}

func parseGithubURL(path string) (org, repo, release, file string) {
	splitted := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(splitted) != 6 {
		return "", "", "", ""
	} else if splitted[2] != "releases" {
		return "", "", "", ""
	} else if (splitted[3] != "latest" || splitted[4] != "download") && splitted[3] != "download" {
		return "", "", "", ""
	}

	if splitted[3] == "latest" {
		return splitted[0], splitted[1], "", splitted[5]
	}

	return splitted[0], splitted[1], splitted[4], splitted[5]
}
