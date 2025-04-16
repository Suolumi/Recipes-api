package config

import (
	"recipes/internal/utils"
	"time"
)

type JwtConfig struct {
	AccessSecret      string
	AccessExpiration  time.Duration
	RefreshSecret     string
	RefreshExpiration time.Duration
}

type DatabaseConfig struct {
	DefaultAddr   string
	Name          string
	Timeout       time.Duration
	AdminUsername string
	AdminMail     string
	AdminPassword string
}

type RuntimeConfig struct {
	Port           int
	WebappUrl      string
	ImagesDir      string
	RecipeImageDir string
}

type LruConfig struct {
	RecipeImageTimeout time.Duration
}

type Config struct {
	Db  *DatabaseConfig
	Jwt *JwtConfig
	Cfg *RuntimeConfig
	Lru *LruConfig
}

func NewConfig(prefix ...string) (*Config, error) {
	cfg := Config{
		Db: &DatabaseConfig{
			DefaultAddr:   "mongodb://localhost:27017",
			Name:          "Recipes",
			Timeout:       time.Second * 2,
			AdminUsername: "admin",
			AdminMail:     "admin@admin.admin",
			AdminPassword: "admin",
		},
		Jwt: &JwtConfig{
			AccessSecret:      "",
			AccessExpiration:  time.Hour,
			RefreshSecret:     "",
			RefreshExpiration: time.Hour * 24 * 7 * 30,
		},
		Cfg: &RuntimeConfig{
			Port:           8080,
			WebappUrl:      "http://localhost:5173",
			ImagesDir:      "assets/pictures",
			RecipeImageDir: "assets/recipeImages",
		},
		Lru: &LruConfig{
			RecipeImageTimeout: time.Hour,
		},
	}

	err := utils.LoadConfigFromEnv(&cfg, prefix...)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}
