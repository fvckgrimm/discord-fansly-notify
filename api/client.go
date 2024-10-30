package api

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	HTTPClient *http.Client
	BaseURL    string
	Token      string
	UserAgent  string
	DeviceID   string
	SessionID  string
	CheckKey   string
}

type AccountInfo struct {
	ID string `json:"id"`
}

type ModelAccountInfo struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Avatar   struct {
		Variants []struct {
			Locations []struct {
				Location string `json:"location"`
			} `json:"locations"`
		} `json:"variants"`
		Locations []struct {
			Location string `json:"location"`
		} `json:"locations"`
	} `json:"avatar"`
}

type FollowingAccount struct {
	AccountID string `json:"accountId"`
}

type FanslyError struct {
	Code    int    `json:"code"`
	Details string `json:"details"`
}

type FanslyResponse struct {
	Success bool        `json:"success"`
	Error   FanslyError `json:"error"`
}

func NewClient(token, userAgent string) (*Client, error) {
	client := &Client{
		HTTPClient: &http.Client{},
		BaseURL:    "https://apiv3.fansly.com",
		Token:      token,
		UserAgent:  userAgent,
	}

	deviceID, err := client.getDeviceID()
	if err != nil {
		return nil, err
	}
	client.DeviceID = deviceID

	sessionID, err := client.getSessionID()
	if err != nil {
		return nil, err
	}
	client.SessionID = sessionID

	checkKey, err := client.guessCheckKey()
	if err != nil {
		client.CheckKey = "oybZy8-fySzis-bubayf"
	} else {
		client.CheckKey = checkKey
	}

	//fmt.Printf("[NewClient] Client: %v\n", client)
	return client, nil
}

func (c *Client) sendRequest(req *http.Request) (*http.Response, error) {
	// Essential Fansly headers
	headers := map[string]string{
		"authorization":       c.Token,
		"fansly-client-check": c.getFanslyClientCheck(req.URL.String()),
		"fansly-client-id":    c.DeviceID,
		"fansly-client-ts":    fmt.Sprintf("%d", getClientTimestamp()),
		"fansly-session-id":   c.SessionID,
		"origin":              "https://fansly.com",
		"referer":             "https://fansly.com/",
		"user-agent":          c.UserAgent,
	}

	// Apply all headers to the request
	for key, value := range headers {
		req.Header.Set(key, value)
		//fmt.Printf("%s : %s ", key, value)
	}
	//fmt.Printf("[sendRequest] Headers: %v\n", headers)
	return c.HTTPClient.Do(req)
}

func (c *Client) GetMyAccountInfo() (*AccountInfo, error) {
	url := fmt.Sprintf("%s/api/v1/account/me?ngsw-bypass=true", c.BaseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	//req.Header.Set("Authorization", c.Token)
	//req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.sendRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Success  bool `json:"success"`
		Response struct {
			Account AccountInfo `json:"account"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	//fmt.Printf("%v", resp)

	if !result.Success {
		//fmt.Printf("%v", result)
		return nil, fmt.Errorf("failed to get account info")
	}

	if result.Response.Account.ID == "" {
		fmt.Printf("Warning: Empty account ID returned\n")
	}

	return &result.Response.Account, nil
}

func (c *Client) GetFollowing(accountID string) ([]FollowingAccount, error) {
	url := fmt.Sprintf("%s/api/v1/account/%s/following?before=0&after=0&limit=999&offset=0", c.BaseURL, accountID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.sendRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Success  bool               `json:"success"`
		Response []FollowingAccount `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if !result.Success {
		//fmt.Printf("%v", url)
		return nil, fmt.Errorf("failed to get following list")
	}

	return result.Response, nil
}

func (c *Client) FollowAccount(modelID string) error {
	url := fmt.Sprintf("%s/api/v1/account/%s/followers?ngsw-bypass=true", c.BaseURL, modelID)
	//fmt.Printf("[FollowAccount] URL: %v\n", url)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.sendRequest(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result FanslyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if !result.Success {
		return fmt.Errorf("failed to follow account (code %d): %s", result.Error.Code, result.Error.Details)
	}

	return nil
}

func (c *Client) getFanslyClientCheck(reqURL string) string {
	parsedURL, _ := url.Parse(reqURL)
	urlPath := parsedURL.Path
	uniqueIdentifier := fmt.Sprintf("%s_%s_%s", c.CheckKey, urlPath, c.DeviceID)
	digest := cyrb53(uniqueIdentifier)
	return fmt.Sprintf("%x", digest)
}

func getClientTimestamp() int64 {
	now := time.Now().UnixNano() / int64(time.Millisecond)
	randomValue, _ := rand.Int(rand.Reader, big.NewInt(10000))
	return now + (5000 - randomValue.Int64())
}
