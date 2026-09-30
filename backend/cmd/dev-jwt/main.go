package main

import (
	"fmt"
	"os"
	"time"
	"github.com/google/uuid"
	"github.com/halal-equity/backend/internal/auth"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load("../../.env")
	
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "change-this-to-a-long-random-jwt-secret"
	}

	jwtManager := auth.NewJWTManager(secret, 8760 * time.Hour) // 1 year expiry for dev

	// Fixed DEV user ID
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	token, err := jwtManager.GenerateToken(userID, "dev@halalequity.local", "Developer")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(token)
}
