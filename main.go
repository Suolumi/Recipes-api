package main

import (
	"log"
	"recipes/cmd/config"
	"recipes/internal/handlers"
)

// @title           Recipes API Swagger
// @version         1.0
// @description     Swagger

// @host      localhost:8080
// @securityDefinitions.basic  BasicAuth
func main() {
	cfg, err := config.NewConfig("RECIPES")
	if err != nil {
		log.Fatal(err)
	}

	hdl, err := handlers.New(cfg)
	if err != nil {
		log.Fatal(err)
	}

	hdl.RegisterEndpoints()
	hdl.Run(cfg.Cfg.Port)
}
