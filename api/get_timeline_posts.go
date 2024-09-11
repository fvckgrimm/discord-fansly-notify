package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	//"time"
)

type Post struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"createdAt"`
}

type TimelineResponse struct {
	Success  bool `json:"success"`
	Response struct {
		Posts                       []Post `json:"posts"`
		TimelineReadPermissionFlags []struct {
			ID        string `json:"id"`
			AccountID string `json:"accountId"`
			Type      int    `json:"type"`
			Flags     int    `json:"flags"`
			Metadata  string `json:"metadata"`
		} `json:"timelineReadPermissionFlags"`
		AccountTimelineReadPermissionFlags struct {
			Flags    int    `json:"flags"`
			Metadata string `json:"metadata"`
		} `json:"accountTimelineReadPermissionFlags"`
	} `json:"response"`
}

func hasTimelineAccess(response TimelineResponse) bool {
	requiredFlags := response.Response.TimelineReadPermissionFlags
	userFlags := response.Response.AccountTimelineReadPermissionFlags.Flags

	// If TimelineReadPermissionFlags is empty, everyone has access
	if len(requiredFlags) == 0 {
		return true
	}

	// Check if user's flags match any of the required flags
	for _, flag := range requiredFlags {
		if userFlags&flag.Flags != 0 {
			return true
		}
	}

	return false
}

func (c *Client) GetTimelinePost(modelID string) ([]Post, error) {
	before := "0"
	url := fmt.Sprintf("https://apiv3.fansly.com/api/v1/timelinenew/%s?before=%s&after=0&wallId&contentSearch&ngsw-bypass=true", modelID, before)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	resp, err := c.sendRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	var timelineResp TimelineResponse
	err = json.NewDecoder(resp.Body).Decode(&timelineResp)
	if err != nil {
		return nil, err
	}

	if !hasTimelineAccess(timelineResp) {
		return nil, fmt.Errorf("no timeline access for user %s", modelID)
	}

	/*
		isLive := streamResp.Success &&
			streamResp.Response.Stream.Status == 2 &&
			//streamResp.Response.Stream.Access &&
			time.Now().UnixMilli()-streamResp.Response.Stream.LastFetchedAt < 5*60*1000 // Last fetched within 5 minutes
	*/

	return timelineResp.Response.Posts, nil
}

func getTimelinePostsBatch(modelId, before string, authToken string, userAgent string) (TimelineResponse, string, error) {
	headerMap := map[string]string{
		"Authorization": authToken,
		"User-Agent":    userAgent,
	}
	client := &http.Client{}
	url := fmt.Sprintf("https://apiv3.fansly.com/api/v1/timelinenew/%s?before=%s&after=0&wallId&contentSearch&ngsw-bypass=true", modelId, before)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return TimelineResponse{}, "", err
	}

	//headers.AddHeadersToRequest(req, true)
	for key, value := range headerMap {
		req.Header.Add(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return TimelineResponse{}, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return TimelineResponse{}, "", fmt.Errorf("failed to fetch model timeline with status code %d", resp.StatusCode)
	}

	var timelineResp TimelineResponse
	err = json.NewDecoder(resp.Body).Decode(&timelineResp)
	if err != nil {
		return TimelineResponse{}, "", err
	}

	//if timelineResp.Response.AccountTimelineReadPermissionFlags.Flags == 0 {
	//    return nil, "", fmt.Errorf("not subscribed: unable to get timeline feed")
	//}

	if len(timelineResp.Response.Posts) == 0 {
		return timelineResp, "", nil
	}

	//nextBefore := posts[len(posts)-1].ID
	nextBefore := timelineResp.Response.Posts[len(timelineResp.Response.Posts)-1].ID
	//log.Printf("[Timeline Batch] Last Post Id in resposne: %v", nextBefore)

	return timelineResp, nextBefore, nil

}
