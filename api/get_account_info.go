package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

//type AccountInfo struct {
//	ID       string `json:"id"`
//	Username string `json:"username"`
//	// Add other fields as needed
//}

func (c *Client) GetAccountInfo(username string) (*ModelAccountInfo, error) {
	url := fmt.Sprintf("%s/api/v1/account?usernames=%s&ngsw-bypass=true", c.BaseURL, username)
	//fmt.Printf("Creator Request Url: %v\n", url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.sendRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	//fmt.Printf("Creator Response: %v", resp)

	var result struct {
		Success  bool               `json:"success"`
		Response []ModelAccountInfo `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if !result.Success || len(result.Response) == 0 {
		return nil, fmt.Errorf("failed to get account info for %s", username)
	}

	if len(result.Response) == 0 {
		return nil, fmt.Errorf("no account info found for %s", username)
	}

	return &result.Response[0], nil
}
