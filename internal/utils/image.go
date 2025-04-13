package utils

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"math"
	"net/http"

	"github.com/fogleman/gg"
	"github.com/nfnt/resize"
)

func GenerateSignCard(backgroundImg []byte, avatarURL, nickname string, signDays int) ([]byte, error) {
	background, err := decodeImage(backgroundImg)
	if err != nil {
		return nil, fmt.Errorf("failed to decode background image: %v", err)
	}

	bgWidth := background.Bounds().Dx()
	bgHeight := background.Bounds().Dy()

	// automatically calculate a suitable ratio based on the background image size
	// default reference size is about 300x200 pixels
	scaleRatio := float64(bgWidth) / 300.0

	log.Printf("background image size: %dx%d, calculated scale ratio: %.2f", bgWidth, bgHeight, scaleRatio)

	dc := gg.NewContext(bgWidth, bgHeight)

	dc.DrawImage(background, 0, 0)

	avatarSize := int(30 * scaleRatio)
	avatarX := int(33 * scaleRatio)
	avatarY := int(161 * scaleRatio)

	log.Printf("avatar parameters - size: %d, position: (%d, %d)", avatarSize, avatarX, avatarY)

	avatarImg, err := loadAndResizeAvatar(avatarURL, uint(avatarSize))
	if err != nil {
		log.Printf("failed to load avatar: %v, using default avatar", err)
	} else {
		dc.DrawImage(avatarImg, avatarX, avatarY)
	}

	nicknameSize := int(12 * scaleRatio)
	nicknameX := int(68 * scaleRatio)
	nicknameY := int(179*scaleRatio) + int(13*scaleRatio)
	nicknameRotate := 10.0
	nicknameRotateX := int((68 + 45) * scaleRatio)
	nicknameRotateY := int((179+6)*scaleRatio) + int(13*scaleRatio)

	log.Printf("nickname parameters - font size: %d, position: (%d, %d), rotation center: (%d, %d)",
		nicknameSize, nicknameX, nicknameY, nicknameRotateX, nicknameRotateY)

	if err := dc.LoadFontFace("assets/fonts/MiSans/MiSans-Bold.ttf", float64(nicknameSize)); err != nil {
		return nil, fmt.Errorf("failed to load font: %v", err)
	}

	// draw nickname
	dc.SetRGB(1, 1, 1)
	dc.RotateAbout(nicknameRotate*(math.Pi/180), float64(nicknameRotateX), float64(nicknameRotateY))
	dc.DrawString(nickname, float64(nicknameX), float64(nicknameY))
	dc.RotateAbout(-nicknameRotate*(math.Pi/180), float64(nicknameRotateX), float64(nicknameRotateY))

	// draw sign days
	daysSize := int(16 * scaleRatio)
	daysX := int(201 * scaleRatio)
	daysY := int(161*scaleRatio) + int(12*scaleRatio)
	daysRotate := -11.0
	daysRotateX := int((201 + 50) * scaleRatio)
	daysRotateY := int((161+8)*scaleRatio) + int(12*scaleRatio)

	log.Printf("sign days parameters - font size: %d, position: (%d, %d), rotation center: (%d, %d)",
		daysSize, daysX, daysY, daysRotateX, daysRotateY)

	if err := dc.LoadFontFace("assets/fonts/MiSans/MiSans-Bold.ttf", float64(daysSize)); err != nil {
		return nil, fmt.Errorf("failed to load font: %v", err)
	}

	daysText := fmt.Sprintf("%d", signDays)
	dc.SetRGB(1, 1, 1)
	dc.RotateAbout(daysRotate*(math.Pi/180), float64(daysRotateX), float64(daysRotateY))
	textWidth, _ := dc.MeasureString(daysText)
	dc.DrawString(daysText, float64(daysRotateX)-textWidth/2, float64(daysY)+float64(daysSize)/3)

	var buf bytes.Buffer
	if err := dc.EncodePNG(&buf); err != nil {
		return nil, fmt.Errorf("failed to encode image: %v", err)
	}

	return buf.Bytes(), nil
}

func GenerateUserCard(nickname, sign, avatar string, uid, exp, contribution int, location, birthday string) (string, error) {
	dc := gg.NewContext(800, 450)

	dc.SetHexColor("#f5f5f5")
	dc.Clear()

	dc.SetHexColor("#ffffff")
	dc.DrawRoundedRectangle(20, 20, 760, 410, 10)
	dc.Fill()

	avatarImg, err := loadAndResizeAvatar(avatar, 120)
	if err == nil {
		dc.DrawImage(avatarImg, 40, 40)
	}

	if err := dc.LoadFontFace("assets/fonts/MiSans/MiSans-Medium.ttf", 28); err != nil {
		return "", fmt.Errorf("failed to load font: %v", err)
	}

	dc.SetHexColor("#333333")
	dc.DrawString(nickname, 180, 80)
	dc.LoadFontFace("assets/fonts/MiSans/MiSans-Regular.ttf", 18)
	dc.SetHexColor("#999999")
	dc.DrawString(fmt.Sprintf("UID: %d", uid), 180, 110)

	dc.LoadFontFace("assets/fonts/MiSans/MiSans-Regular.ttf", 20)
	dc.SetHexColor("#666666")
	dc.DrawString(sign, 40, 200)

	dc.LoadFontFace("assets/fonts/MiSans/MiSans-Medium.ttf", 22)
	dc.SetHexColor("#333333")
	dc.DrawString(fmt.Sprintf("经验值: %d", exp), 40, 280)
	dc.DrawString(fmt.Sprintf("贡献值: %d", contribution), 40, 320)
	dc.DrawString(fmt.Sprintf("地区: %s", location), 40, 360)
	dc.DrawString(fmt.Sprintf("生日: %s", birthday), 40, 400)

	var buf bytes.Buffer
	if err := dc.EncodePNG(&buf); err != nil {
		return "", fmt.Errorf("failed to encode image: %v", err)
	}

	return "base64://" + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func loadAndResizeAvatar(avatarURL string, size uint) (image.Image, error) {
	log.Printf("start downloading avatar: %s", avatarURL)

	req, err := http.NewRequest("GET", avatarURL, nil)
	if err != nil {
		log.Printf("failed to create request: %v", err)
		return nil, err
	}
	req.Header.Set("Referer", "https://2550505.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("failed to download avatar: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	log.Printf("download avatar HTTP status: %d, Content-Type: %s", resp.StatusCode, resp.Header.Get("Content-Type"))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download avatar, HTTP status code: %d", resp.StatusCode)
	}

	imgData, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("failed to read avatar data: %v", err)
		return nil, err
	}

	log.Printf("successfully downloaded avatar, data size: %d bytes", len(imgData))

	contentType := http.DetectContentType(imgData)
	log.Printf("detected image type: %s", contentType)

	img, err := decodeImage(imgData)
	if err != nil {
		log.Printf("failed to decode avatar: %v", err)
		return nil, err
	}

	log.Printf("successfully decoded avatar, size: %dx%d", img.Bounds().Dx(), img.Bounds().Dy())

	resized := resize.Resize(size, size, img, resize.Lanczos3)
	log.Printf("resized avatar to %dx%d", size, size)

	return createCircularAvatar(resized), nil
}

func decodeImage(data []byte) (image.Image, error) {
	contentType := http.DetectContentType(data)

	switch contentType {
	case "image/jpeg":
		img, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			log.Printf("failed to decode JPEG: %v", err)
			return nil, err
		}
		return img, nil

	case "image/png":
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			log.Printf("failed to decode PNG: %v", err)
			return nil, err
		}
		return img, nil

	default:
		img, err := jpeg.Decode(bytes.NewReader(data))
		if err == nil {
			return img, nil
		}

		img, err = png.Decode(bytes.NewReader(data))
		if err == nil {
			return img, nil
		}

		return nil, fmt.Errorf("unsupported image type: %s", contentType)
	}
}

func createCircularAvatar(img image.Image) image.Image {
	size := img.Bounds().Size()
	dc := gg.NewContext(size.X, size.Y)

	dc.DrawCircle(float64(size.X)/2, float64(size.Y)/2, float64(size.X)/2)
	dc.Clip()

	dc.DrawImage(img, 0, 0)

	return dc.Image()
}
