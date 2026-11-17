package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo      *Repository
	apiKey    string
	jwtSecret []byte
	client    *http.Client
}

func NewService(repo *Repository, apiKey, jwtSecret string) *Service {
	return &Service{
		repo:      repo,
		apiKey:    apiKey,
		jwtSecret: []byte(jwtSecret),
		client:    &http.Client{Timeout: 45 * time.Second},
	}
}

func (s *Service) Register(username, password, preferences string) error {
	if username == "" || password == "" {
		return errors.New("имя пользователя и пароль обязательны")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.repo.CreateUser(username, string(hash), preferences)
	return err
}

func (s *Service) Login(username, password string) (string, error) {
	user, err := s.repo.GetUserByUsername(username)
	if err != nil {
		return "", errors.New("неверный логин или пароль")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", errors.New("неверный логин или пароль")
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  user.ID,
		"username": user.Username,
		"exp":      time.Now().Add(72 * time.Hour).Unix(),
	})
	return token.SignedString(s.jwtSecret)
}

func (s *Service) ParseToken(tokenStr string) (int, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return s.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return 0, errors.New("невалидный токен")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, errors.New("ошибка чтения токена")
	}
	userIDFloat, ok := claims["user_id"].(float64)
	if !ok {
		return 0, errors.New("user_id не найден в токене")
	}
	return int(userIDFloat), nil
}

func (s *Service) GenerateRecipe(userID int, mode string, items []string, dishName string) (string, error) {
	var prompt string
	var queryLog string

	switch mode {
	case "ingredients":
		if len(items) == 0 {
			return "", errors.New("передайте список продуктов в поле ingredients")
		}
		joined := strings.Join(items, ", ")
		queryLog = "Продукты: " + joined
		prompt = fmt.Sprintf(
			"Ты профессиональный шеф-повар. Составь подробный рецепт блюда, используя следующие продукты: %s. "+
				"Укажи название блюда, точные пропорции, калорийность и пошаговую инструкцию приготовления на русском языке.",
			joined,
		)
	case "dish_name":
		if strings.TrimSpace(dishName) == "" {
			return "", errors.New("передайте название блюда в поле dish_name")
		}
		queryLog = "Блюдо: " + dishName
		prompt = fmt.Sprintf(
			"Ты профессиональный шеф-повар. Напиши классический подробный рецепт блюда «%s». "+
				"Укажи список необходимых ингредиентов, время готовки, калорийность и пошаговую инструкцию на русском языке.",
			dishName,
		)
	default:
		return "", errors.New("неверный тип запроса: используйте mode = 'ingredients' или 'dish_name'")
	}

	result, err := s.callGeminiText(prompt)
	if err != nil {
		return "", err
	}

	_ = s.repo.SaveHistory(userID, mode, queryLog, result)
	return result, nil
}

func (s *Service) GetRecipeOfTheDay() (string, error) {
	today := time.Now().Format("2006-01-02")
	cached, err := s.repo.GetDailyRecipe(today)
	if err == nil && cached != "" {
		return cached, nil
	}

	prompt := fmt.Sprintf(
		"Предложи интересный, вкусный и сбалансированный «Рецепт дня» на дату %s. "+
			"Включи оригинальное название, список доступных ингредиентов, КБЖУ и пошаговый план приготовления на русском языке.",
		today,
	)
	recipe, err := s.callGeminiText(prompt)
	if err != nil {
		return "", err
	}

	_ = s.repo.SaveDailyRecipe(today, recipe)
	return recipe, nil
}

func (s *Service) GetRecommendations(userID int) (string, error) {
	user, err := s.repo.GetUserByID(userID)
	if err != nil {
		return "", err
	}
	recentQueries, _ := s.repo.GetUserRecentQueries(userID, 5)

	historyContext := "История запросов пока пуста."
	if len(recentQueries) > 0 {
		historyContext = "Последние запросы пользователя: " + strings.Join(recentQueries, "; ")
	}

	prefs := user.Preferences
	if prefs == "" {
		prefs = "нет особых предпочтений"
	}

	prompt := fmt.Sprintf(
		"Составь 3 персональные кулинарные рекомендации (название блюда, краткое описание и почему оно подойдет) для пользователя. "+
			"Предпочтения пользователя: %s. %s. Ответь на русском языке.",
		prefs, historyContext,
	)

	return s.callGeminiText(prompt)
}

type ImageResponse struct {
	DishName        string `json:"dish_name"`
	Base64Image     string `json:"base64_image,omitempty"`
	MimeType        string `json:"mime_type,omitempty"`
	ImageURL        string `json:"image_url"`
	VisualPromptEng string `json:"visual_prompt_eng"`
}

func (s *Service) GenerateOrSearchImage(dishQuery string) (*ImageResponse, error) {
	if strings.TrimSpace(dishQuery) == "" {
		return nil, errors.New("укажите название блюда (dish_name)")
	}

	prompt := fmt.Sprintf(
		"Create a concise, highly detailed English food photography prompt (max 25 words) for the dish: '%s'. "+
			"Return ONLY the prompt text without quotes or explanations.",
		dishQuery,
	)
	visualPrompt, err := s.callGeminiText(prompt)
	if err != nil {
		visualPrompt = dishQuery + " gourmet food photography, high resolution"
	}
	visualPrompt = strings.TrimSpace(visualPrompt)

	base64Img, mimeType, imgErr := s.callGeminiImagen(visualPrompt)

	fallbackURL := fmt.Sprintf("https://image.pollinations.ai/prompt/%s?width=800&height=600&nologo=true", url.PathEscape(visualPrompt))

	resp := &ImageResponse{
		DishName:        dishQuery,
		ImageURL:        fallbackURL,
		VisualPromptEng: visualPrompt,
	}
	if imgErr == nil && base64Img != "" {
		resp.Base64Image = base64Img
		resp.MimeType = mimeType
	}

	return resp, nil
}

func (s *Service) callGeminiText(prompt string) (string, error) {
	endpoint := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-3.5-flash-lite:generateContent?key=%s", s.apiKey)

	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]string{
					{"text": prompt},
				},
			},
		},
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respData, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ошибка Gemini API (%d): %s", resp.StatusCode, string(respData))
	}

	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(respData, &parsed); err != nil {
		return "", err
	}

	if len(parsed.Candidates) > 0 && len(parsed.Candidates[0].Content.Parts) > 0 {
		return parsed.Candidates[0].Content.Parts[0].Text, nil
	}
	return "", errors.New("пустой ответ от Gemini API")
}

func (s *Service) callGeminiImagen(prompt string) (string, string, error) {
	endpoint := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/imagen-3.0-generate-002:predict?key=%s", s.apiKey)

	payload := map[string]interface{}{
		"instances": []map[string]string{
			{"prompt": prompt},
		},
		"parameters": map[string]interface{}{
			"sampleCount": 1,
		},
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("imagen status %d", resp.StatusCode)
	}

	var parsed struct {
		Predictions []struct {
			BytesBase64Encoded string `json:"bytesBase64Encoded"`
			MimeType           string `json:"mimeType"`
		} `json:"predictions"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", "", err
	}
	if len(parsed.Predictions) > 0 {
		return parsed.Predictions[0].BytesBase64Encoded, parsed.Predictions[0].MimeType, nil
	}
	return "", "", errors.New("нет сгенерированных изображений")
}
