terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

provider "google" {
  project = "smart-chef-ai-2026"
  region  = "us-central1"
}

# 1. База данных Firestore (NoSQL)
resource "google_firestore_database" "database" {
  name        = "(default)"
  location_id = "nam5"
  type        = "FIRESTORE_NATIVE"
}

# 2. Облачный реестр для Docker-образов
resource "google_artifact_registry_repository" "repo" {
  location      = "us-central1"
  repository_id = "smart-chef-repo"
  description   = "Docker repository for Smart Chef API"
  format        = "DOCKER"
}

# 3. Сервис Cloud Run для бэкенда
resource "google_cloud_run_v2_service" "api" {
  name     = "smart-chef-api"
  location = "us-central1"
  ingress  = "INGRESS_TRAFFIC_ALL"

  template {
    containers {
      # Пока разворачиваем тестовый образ, позже заменим на твой Go-код
      image = "us-central1-docker.pkg.dev/smart-chef-ai-2026/smart-chef-repo/smart-chef-api:v1"
      
      env {
        name  = "GEMINI_API_KEY"
        value = "YOUR_API_KEY_HERE" 
      }
    }
  }
}

# 4. Открываем публичный доступ к нашему API
resource "google_cloud_run_v2_service_iam_member" "public_access" {
  project  = google_cloud_run_v2_service.api.project
  location = google_cloud_run_v2_service.api.location
  name     = google_cloud_run_v2_service.api.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

# 5. Вывод публичной ссылки на API
output "api_url" {
  value = google_cloud_run_v2_service.api.uri
}