package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type Client struct {
	HTTPClient *http.Client
	BaseURL    string
	Token      string
	UserAgent  string
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

func NewClient(token, userAgent string) *Client {
	return &Client{
		HTTPClient: &http.Client{},
		BaseURL:    "https://apiv3.fansly.com",
		Token:      token,
		UserAgent:  userAgent,
	}
}

func (c *Client) sendRequest(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", c.Token)
	req.Header.Set("User-Agent", c.UserAgent)
	//fmt.Printf("URL: %v, TOKEN: %v, AGENT: %v", req, c.Token, c.UserAgent)
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
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.sendRequest(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if !result.Success {
		return fmt.Errorf("failed to follow account")
	}

	return nil
}
