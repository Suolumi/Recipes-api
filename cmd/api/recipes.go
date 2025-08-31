package main

import (
	"log"
	"os"
	"recipes/internal/config"
	"recipes/internal/handlers"
)

// @title           Recipes API Swagger
// @version         1.0
// @description     Swagger

// @host      localhost:8080
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	cfg, err := config.NewConfig("RECIPES")
	if err != nil {
		log.Fatal(err)
	}

	dirs := []string{
		cfg.Cfg.RecipeImageDir,
		cfg.Cfg.ImagesDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatal(err)
		}
	}

	hdl, err := handlers.New(cfg)
	if err != nil {
		log.Fatal(err)
	}

	hdl.RegisterEndpoints()
	hdl.Run(cfg.Cfg.Port)
}
