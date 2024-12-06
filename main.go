package main

import (
	"log"
	"recipes/cmd/config"
	"recipes/internal/handlers"
)

func main() {
	cfg, err := config.NewConfig("RECIPES")
	if err != nil {
		log.Fatal(err)
	}

	hdl, err := handlers.New(cfg)
	if err != nil {
		log.Fatal(err)
	}

	hdl.Init()
	hdl.Run(cfg.Cfg.Port)
}
