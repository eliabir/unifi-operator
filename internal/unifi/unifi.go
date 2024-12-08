/* https://github.com/clbx/kube-port-forward-controller */

package unifi

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"

	"github.com/paultyng/go-unifi/unifi"
)

type UnifiClient struct {
	Client *unifi.Client
	SiteID string
}

func CreateUnifiClient() (*UnifiClient, error) {
	client := &unifi.Client{}

	unifiURL := os.Getenv("UNIFI_URL")
	if unifiURL == "" {
		return nil, errors.New("could not get UniFi URL from environment variables")
	}

	siteID := os.Getenv("UNIFI_SITE")
	if siteID == "" {
		return nil, errors.New("could not get UniFi site ID from environment variables")
	}

	username := os.Getenv("UNIFI_USER")
	if username == "" {
		return nil, errors.New("could not get UniFi username from environment variables")
	}

	password := os.Getenv("UNIFI_PASSWORD")
	if password == "" {
		return nil, errors.New("could not get UniFi password from environment variables")
	}

	if err := client.SetBaseURL(unifiURL); err != nil {
		return nil, err
	}

	jar, _ := cookiejar.New(nil)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := &http.Client{
		Jar:       jar,
		Transport: tr,
	}

	if err := client.SetHTTPClient(httpClient); err != nil {
		return nil, errors.New(fmt.Sprintf("failed to set HTTP client: %s", err))
	}

	if err := client.Login(context.Background(), username, password); err != nil {
		return nil, errors.New(fmt.Sprintf("could not log into UniFi controller: %s", err))
	}

	unifiClient := &UnifiClient{
		Client: client,
		SiteID: siteID,
	}

	return unifiClient, nil
}
