package api

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"io"
	"net/http"
	"regexp"
	"strings"
)

func (c *Client) getDeviceID() (string, error) {
	req, err := http.NewRequest("GET", "https://apiv3.fansly.com/api/v1/device/id", nil)
	if err != nil {
		return "", err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Success  bool   `json:"success"`
		Response string `json:"response"`
	}

	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", err
	}

	if !result.Success {
		return "", fmt.Errorf("failed to get device ID")
	}

	return result.Response, nil
}

func (c *Client) getSessionID() (string, error) {
	wsConn, _, err := websocket.DefaultDialer.Dial("wss://wsv3.fansly.com/", nil)
	if err != nil {
		return "", err
	}
	defer wsConn.Close()

	message := map[string]interface{}{
		"t": 1,
		"d": fmt.Sprintf("{\"token\":\"%s\"}", c.Token),
	}

	err = wsConn.WriteJSON(message)
	if err != nil {
		return "", err
	}

	_, msg, err := wsConn.ReadMessage()
	if err != nil {
		return "", err
	}

	var response struct {
		T int    `json:"t"`
		D string `json:"d"`
	}

	err = json.Unmarshal(msg, &response)
	if err != nil {
		return "", err
	}

	var sessionData struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
	}

	err = json.Unmarshal([]byte(response.D), &sessionData)
	if err != nil {
		return "", err
	}

	return sessionData.Session.ID, nil
}

func (c *Client) guessCheckKey() (string, error) {
	mainJSPattern := `\ssrc\s*=\s*"(main\..*?\.js)"`
	checkKeyPattern := `this\.checkKey_\s*=\s*\["([^"]+)","([^"]+)"\]\.reverse\(\)\.join\("-"\)\+"([^"]+)"`

	req, err := http.NewRequest("GET", "https://fansly.com/", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	mainJSRegex := regexp.MustCompile(mainJSPattern)
	mainJSMatch := mainJSRegex.FindStringSubmatch(string(body))
	if len(mainJSMatch) < 2 {
		return "", fmt.Errorf("main.js file not found")
	}

	mainJSURL := fmt.Sprintf("https://fansly.com/%s", mainJSMatch[1])
	req, err = http.NewRequest("GET", mainJSURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err = c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	jsBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	checkKeyRegex := regexp.MustCompile(checkKeyPattern)
	checkKeyMatch := checkKeyRegex.FindStringSubmatch(string(jsBody))
	if len(checkKeyMatch) < 4 {
		return "", fmt.Errorf("check key not found")
	}

	return strings.Join([]string{checkKeyMatch[2], checkKeyMatch[1]}, "-") + checkKeyMatch[3], nil
}

func cyrb53(str string) uint64 {
	h1 := uint64(0xdeadbeef)
	h2 := uint64(0x41c6ce57)

	for i := 0; i < len(str); i++ {
		ch := uint64(str[i])
		h1 = (h1 ^ ch) * 2654435761
		h2 = (h2 ^ ch) * 1597334677
	}

	h1 = ((h1 ^ (h1 >> 16)) * 2246822507) ^ ((h2 ^ (h2 >> 13)) * 3266489909)
	h2 = ((h2 ^ (h2 >> 16)) * 2246822507) ^ ((h1 ^ (h1 >> 13)) * 3266489909)

	return 4294967296*(h2&0xFFFFFFFF) + (h1 & 0xFFFFFFFF)
}
