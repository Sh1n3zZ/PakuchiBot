package mgclub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{},
	}
}

type UserInfo struct {
	UID            int     `json:"uid"`
	Nickname       string  `json:"nickname"`
	Avatar         string  `json:"avatar"`
	Role           int     `json:"role"`
	Status         int     `json:"status"`
	Exp            int     `json:"exp"`
	Contribution   int     `json:"contribution"`
	Sign           string  `json:"sign"`
	Auth           int     `json:"auth"`
	Authentication string  `json:"authentication"`
	Location       string  `json:"location"`
	ISP            *string `json:"isp"`
	Sex            *int    `json:"sex"`
	Birthday       *int    `json:"birthday"`
}

func (c *Client) GetUserInfo(token string) (*UserInfo, error) {
	req, err := http.NewRequest("GET", "https://2550505.com/user/info", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Cookie", "token="+token)
	req.Header.Set("Authorization", "114-514-1919-810")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	var result struct {
		Code int      `json:"code"`
		Info UserInfo `json:"info"`
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: HTTP status code %d, response body: %s, error: %v",
			resp.StatusCode, string(bodyBytes), err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("API returned error code: %d, HTTP status code: %d, response body: %s",
			result.Code, resp.StatusCode, string(bodyBytes))
	}

	return &result.Info, nil
}

func (u *UserInfo) ParseBirthday() string {
	if u.Birthday == nil {
		return "未知"
	}
	birthdayTime := time.UnixMilli(int64(*u.Birthday))
	return fmt.Sprintf("%d 年 %d 月 %d 日", birthdayTime.Year(), birthdayTime.Month(), birthdayTime.Day())
}
