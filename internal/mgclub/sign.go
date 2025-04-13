package mgclub

import (
	"PakuchiBot/internal/utils"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

type SignResponse struct {
	Code       int    `json:"code" protobuf:"varint,1,opt,name=code"`
	Exp        int    `json:"exp" protobuf:"varint,2,opt,name=exp"`
	IsBirthday bool   `json:"isBirthday" protobuf:"varint,3,opt,name=isBirthday"`
	Msg        string `json:"msg" protobuf:"bytes,4,opt,name=msg"`
}

type SignDaysResponse struct {
	Code  int  `json:"code" protobuf:"varint,1,opt,name=code"`
	Day   int  `json:"day" protobuf:"varint,2,opt,name=day"`
	Cache bool `json:"cache" protobuf:"varint,3,opt,name=cache"`
}

type SignResult struct {
	IsSuccess   bool
	AlreadyDone bool
	Message     string
	ImageURL    string
	ImageData   []byte
}

func ProcessSign(token string) (SignResult, error) {
	result := SignResult{
		ImageURL: "https://cdn.2550505.com/share/assets/mgclub/editor/sign-1.png",
	}

	client := NewClient()
	userInfo, err := client.GetUserInfo(token)
	if err != nil {
		log.Printf("Failed to get user info: %v", err)
	}

	signResp, err := DoSign(token)
	if err != nil {
		return result, fmt.Errorf("failed to sign in: %w", err)
	}

	if signResp == nil {
		result.AlreadyDone = true
		daysResp, err := GetSignDays(token)
		if err != nil {
			result.Message = "你今天已经签到过了喵～（获取签到天数失败）"
			result.IsSuccess = true

			if userInfo != nil {
				downloadSignCardWithUserInfo(&result, userInfo, 0)
			} else {
				downloadImage(&result)
			}

			return result, nil
		}
		result.Message = fmt.Sprintf("你今天已经签到过了喵～\n当前已连续签到：%d天", daysResp.Day)
		result.IsSuccess = true

		if userInfo != nil {
			downloadSignCardWithUserInfo(&result, userInfo, daysResp.Day)
		} else {
			downloadImage(&result)
		}

		return result, nil
	}

	result.IsSuccess = true
	daysResp, err := GetSignDays(token)
	if err != nil {
		result.Message = fmt.Sprintf("签到成功喵\n获得经验：%d\n（获取签到天数失败：%v）", signResp.Exp, err)

		if userInfo != nil {
			downloadSignCardWithUserInfo(&result, userInfo, 0)
		} else {
			downloadImage(&result)
		}

		return result, nil
	}

	result.Message = fmt.Sprintf("签到成功喵\n获得经验：%d\n已连续签到：%d天", signResp.Exp, daysResp.Day)
	if signResp.IsBirthday {
		result.Message += "\n🎂 生日快乐喵！"
	}
	if signResp.Msg != "" {
		result.Message += fmt.Sprintf("\n消息：%s", signResp.Msg)
	}

	if userInfo != nil {
		downloadSignCardWithUserInfo(&result, userInfo, daysResp.Day)
	} else {
		downloadImage(&result)
	}

	return result, nil
}

func downloadSignCardWithUserInfo(result *SignResult, userInfo *UserInfo, signDays int) {
	if result.ImageURL == "" {
		return
	}

	backgroundImgData, err := DownloadSignImage(result.ImageURL)
	if err != nil {
		log.Printf("Failed to download sign-in image: %v", err)
		result.ImageData = nil
		return
	}

	cardImgData, err := utils.GenerateSignCard(backgroundImgData, userInfo.Avatar, userInfo.Nickname, signDays)
	if err != nil {
		log.Printf("Failed to generate sign card: %v", err)
		result.ImageData = backgroundImgData
		return
	}

	result.ImageData = cardImgData
}

func downloadImage(result *SignResult) {
	if result.ImageURL == "" {
		return
	}

	imageData, err := DownloadSignImage(result.ImageURL)
	if err != nil {
		log.Printf("Failed to download sign-in image: %v", err)
		result.ImageData = nil
		return
	}

	result.ImageData = imageData
}

func DoSign(token string) (*SignResponse, error) {
	client := &http.Client{}
	req, err := http.NewRequest("POST", "https://2550505.com/sign", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Cookie", "token="+token)
	req.Header.Set("Authorization", "114-514-1919-810")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned non-200 status code: %d, response: %s", resp.StatusCode, string(body))
	}

	var signResp SignResponse
	if err := json.Unmarshal(body, &signResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w, response body: %s", err, string(body))
	}

	if signResp.Code == 104 {
		return nil, nil
	}

	return &signResp, nil
}

func GetSignDays(token string) (*SignDaysResponse, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", "https://2550505.com/sign/days", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Cookie", "token="+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var daysResp SignDaysResponse
	if err := json.Unmarshal(body, &daysResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w, response body: %s", err, string(body))
	}

	return &daysResp, nil
}

func DownloadSignImage(imageURL string) ([]byte, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", imageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	req.Header.Set("Referer", "https://2550505.com/")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned non-200 status code: %d", resp.StatusCode)
	}

	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read image data: %w", err)
	}

	return imageData, nil
}
