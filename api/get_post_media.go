package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	//"time"
)

type AccountMediaBundles struct {
	ID              string   `json:"id"`
	Access          bool     `json:"access"`
	AccountMediaIDs []string `json:"accountMediaIds"`
	BundleContent   []struct {
		AccountMediaID string `json:"AccountMediaId"`
		Pos            int    `json:"pos"`
	} `json:"bundleContent"`
}

type Location struct {
	Location string            `json:"location"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type MediaItem struct {
	ID       string `json:"id"`
	Type     int    `json:"type"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Mimetype string `json:"mimetype"`
	Variants []struct {
		ID        string     `json:"id"`
		Type      int        `json:"type"`
		Width     int        `json:"width"`
		Height    int        `json:"height"`
		Mimetype  string     `json:"mimetype"`
		Locations []Location `json:"locations"`
	} `json:"variants"`
	Locations []Location `json:"locations"`
}

type AccountMedia struct {
	ID      string     `json:"id"`
	Media   MediaItem  `json:"media"`
	Preview *MediaItem `json:"preview,omitempty"` // Added to handle optional preview
}

type PostResponse struct {
	Success  bool `json:"success"`
	Response struct {
		Posts []struct {
			ID          string `json:"id"`
			Attachments []struct {
				ContentType int    `json:"contentType"`
				ContentID   string `json:"contentId"`
			} `json:"attachments"`
		} `json:"posts"`
		AccountMediaBundles []AccountMediaBundles `json:"accountMediaBundles"`
		AccountMedia        []AccountMedia        `json:"accountMedia"`
	} `json:"response"`
}

func (c *Client) GetPostMedia(postID, authToken, userAgent string) ([]AccountMedia, error) {
	url := fmt.Sprintf("https://apiv3.fansly.com/api/v1/post?ids=%s&ngsw-bypass=true", postID)

	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", authToken)
	req.Header.Add("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch post with status code %d", resp.StatusCode)
	}

	var postResp PostResponse
	err = json.NewDecoder(resp.Body).Decode(&postResp)
	if err != nil {
		return nil, err
	}

	return postResp.Response.AccountMedia, nil
}
