package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type contextKey string

const userIDKey contextKey = "user_id"

type MainController struct {
	service *Service
}

func NewMainController(service *Service) *MainController {
	return &MainController{service: service}
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (c *MainController) CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (c *MainController) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "требуется заголовок Authorization: Bearer <token>"})
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		userID, err := c.service.ParseToken(token)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
			return
		}
		ctx := context.WithValue(r.Context(), userIDKey, userID)
		next(w, r.WithContext(ctx))
	}
}

func (c *MainController) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		Preferences string `json:"preferences"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "некорректный JSON"})
		return
	}
	if err := c.service.Register(req.Username, req.Password, req.Preferences); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"message": "пользователь успешно зарегистрирован"})
}

func (c *MainController) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "некорректный JSON"})
		return
	}
	token, err := c.service.Login(req.Username, req.Password)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (c *MainController) GenerateRecipe(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(userIDKey).(int)
	var req struct {
		Mode        string   `json:"mode"`
		Ingredients []string `json:"ingredients"`
		DishName    string   `json:"dish_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "некорректный JSON"})
		return
	}

	recipe, err := c.service.GenerateRecipe(userID, req.Mode, req.Ingredients, req.DishName)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"recipe": recipe})
}

func (c *MainController) GetRecipeOfTheDay(w http.ResponseWriter, r *http.Request) {
	recipe, err := c.service.GetRecipeOfTheDay()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"recipe_of_the_day": recipe})
}

func (c *MainController) GetRecommendations(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(userIDKey).(int)
	recommendations, err := c.service.GetRecommendations(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"recommendations": recommendations})
}

func (c *MainController) GenerateOrSearchImage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DishName string `json:"dish_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "некорректный JSON"})
		return
	}

	imgData, err := c.service.GenerateOrSearchImage(req.DishName)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, imgData)
}

func (c *MainController) SwaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := `<!DOCTYPE html>
<html lang="ru">
<head>
  <meta charset="UTF-8">
  <title>Recipe Backend Swagger UI</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css" />
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js"></script>
  <script>
    window.onload = () => {
      window.ui = SwaggerUIBundle({
        url: '/swagger.json',
        dom_id: '#swagger-ui',
      });
    };
  </script>
</body>
</html>`
	w.Write([]byte(html))
}

func (c *MainController) SwaggerSpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	spec := `{
  "openapi": "3.0.0",
  "info": {
    "title": "Recipe Gemini Backend API",
    "version": "1.0.0",
    "description": "Бэкенд на Go с SQLite и Gemini API (Регистрация, Логин, Генерация рецептов, Рецепт дня, Рекомендации и Фото блюд)"
  },
  "components": {
    "securitySchemes": {
      "bearerAuth": {
        "type": "http",
        "scheme": "bearer",
        "bearerFormat": "JWT"
      }
    }
  },
  "paths": {
    "/api/register": {
      "post": {
        "tags": ["Auth"],
        "summary": "1. Регистрация пользователя",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "username": {"type": "string", "example": "alex"},
                  "password": {"type": "string", "example": "123"},
                  "preferences": {"type": "string", "example": "люблю острое и итальянскую кухню"}
                }
              }
            }
          }
        },
        "responses": {
          "201": {"description": "Успешная регистрация"},
          "400": {"description": "Ошибка валидации"}
        }
      }
    },
    "/api/login": {
      "post": {
        "tags": ["Auth"],
        "summary": "2. Логин (получение JWT токена)",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "username": {"type": "string", "example": "alex"},
                  "password": {"type": "string", "example": "123"}
                }
              }
            }
          }
        },
        "responses": {
          "200": {"description": "Возвращает JWT токен"},
          "401": {"description": "Неверный логин или пароль"}
        }
      }
    },
    "/api/recipes/generate": {
      "post": {
        "tags": ["Recipes"],
        "summary": "3. Генерация рецепта (по продуктам или названию блюда)",
        "security": [{"bearerAuth": []}],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "mode": {
                    "type": "string",
                    "enum": ["ingredients", "dish_name"],
                    "example": "ingredients"
                  },
                  "ingredients": {
                    "type": "array",
                    "items": {"type": "string"},
                    "example": ["курица", "сливки", "чеснок", "паста"]
                  },
                  "dish_name": {
                    "type": "string",
                    "example": "Паста Карбонара"
                  }
                }
              }
            }
          }
        },
        "responses": {
          "200": {"description": "Сгенерированный рецепт"},
          "401": {"description": "Не авторизован"}
        }
      }
    },
    "/api/recipes/daily": {
      "get": {
        "tags": ["Recipes"],
        "summary": "4. Генерация рецепта дня",
        "responses": {
          "200": {"description": "Рецепт дня"}
        }
      }
    },
    "/api/recipes/recommendations": {
      "get": {
        "tags": ["Recipes"],
        "summary": "5. Персональные рекомендации для пользователя",
        "security": [{"bearerAuth": []}],
        "responses": {
          "200": {"description": "Список рекомендаций"},
          "401": {"description": "Не авторизован"}
        }
      }
    },
    "/api/recipes/image": {
      "post": {
        "tags": ["Images"],
        "summary": "6. Генерация фото либо поиск в интернете",
        "security": [{"bearerAuth": []}],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "dish_name": {"type": "string", "example": "Паста Карбонара"}
                }
              }
            }
          }
        },
        "responses": {
          "200": {"description": "Ссылка на изображение и промпт"},
          "401": {"description": "Не авторизован"}
        }
      }
    }
  }
}`
	w.Write([]byte(spec))
}
