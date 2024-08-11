package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	//"time"
)

type StreamResponse struct {
	Success  bool `json:"success"`
	Response struct {
		Stream struct {
			Status        int    `json:"status"`
			ViewerCount   int    `json:"viewerCount"`
			LastFetchedAt int64  `json:"lastFetchedAt"`
			StartedAt     int64  `json:"startedAt"`
			PlaybackUrl   string `json:"playbackUrl"`
			Access        bool   `json:"access"`
		} `json:"stream"`
	} `json:"response"`
}

func (c *Client) GetStreamInfo(modelID string) (*StreamResponse, error) {
	url := fmt.Sprintf("https://apiv3.fansly.com/api/v1/streaming/channel/%s", modelID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	resp, err := c.sendRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	var streamResp StreamResponse
	if err := json.NewDecoder(resp.Body).Decode(&streamResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	/*
		isLive := streamResp.Success &&
			streamResp.Response.Stream.Status == 2 &&
			//streamResp.Response.Stream.Access &&
			time.Now().UnixMilli()-streamResp.Response.Stream.LastFetchedAt < 5*60*1000 // Last fetched within 5 minutes
	*/

	return &streamResp, nil
}
