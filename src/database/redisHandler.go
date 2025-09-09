package database

import (
  "github.com/redis/go-redis/v9"
  "fmt"
  "github.com/joho/godotenv"
  "os"
  "log"
)

func LoadRedis() (*redis.Client){
  err := godotenv.Load()
  if err != nil {
      fmt.Println("Error loading .env file")
  }

  opt, err := redis.ParseURL(os.Getenv("REDIS_URL"))
  if err != nil {
      log.Fatalf("Failed to parse Redis URL: %v", err)
  }
  client := redis.NewClient(opt)

  fmt.Println("Redis Client created!")

  return client
}