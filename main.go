package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	_ "modernc.org/sqlite"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Файл .env не найден, используются системные переменные окружения")
	}

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("Ошибка: переменная GEMINI_API_KEY не задана в .env")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "default_secret_key"
	}

	db, err := sql.Open("sqlite", "./recipes.db")
	if err != nil {
		log.Fatal("Ошибка подключения к БД:", err)
	}
	defer db.Close()

	repo := NewRepository(db)
	if err := repo.InitSchema(); err != nil {
		log.Fatal("Ошибка инициализации таблиц БД:", err)
	}

	service := NewService(repo, apiKey, jwtSecret)
	controller := NewMainController(service)

	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/register", controller.Register)
	mux.HandleFunc("POST /api/login", controller.Login)
	mux.HandleFunc("POST /api/recipes/generate", controller.AuthMiddleware(controller.GenerateRecipe))
	mux.HandleFunc("GET /api/recipes/daily", controller.GetRecipeOfTheDay)
	mux.HandleFunc("GET /api/recipes/recommendations", controller.AuthMiddleware(controller.GetRecommendations))
	mux.HandleFunc("POST /api/recipes/image", controller.AuthMiddleware(controller.GenerateOrSearchImage))

	mux.HandleFunc("GET /swagger", controller.SwaggerUI)
	mux.HandleFunc("GET /swagger.json", controller.SwaggerSpec)

	handlerWithCORS := controller.CORSMiddleware(mux)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Сервер запущен на http://localhost:%s", port)
	log.Printf("Swagger UI доступен по адресу: http://localhost:%s/swagger", port)
	if err := http.ListenAndServe(":"+port, handlerWithCORS); err != nil {
		log.Fatal(err)
	}
}
