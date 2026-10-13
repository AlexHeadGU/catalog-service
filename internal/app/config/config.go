package config

import (
	"log"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"

	"github.com/AlexHeadGU/catalog-service/internal/app/config/section"
)

type Config struct {
	Repository section.Repository `split_words:"true"`
	Processor  section.Processor  `split_words:"true"`
	Monitor    section.Monitor    `split_words:"true"`
}

var Root Config

func Load() {
	_ = godotenv.Load()

	if err := envconfig.Process("APP", &Root); err != nil {
		log.Fatal(err)
	}
}
