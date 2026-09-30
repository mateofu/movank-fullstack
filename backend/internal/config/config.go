package config

import (
	"fmt"
	"strconv"
)

type Config struct {
	HTTPPort string
}

func Load(getenv func(string) string) (Config, error) {
	port := getenv("HTTP_PORT")
	if port == "" {
		port = "8080"
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return Config{}, fmt.Errorf("HTTP_PORT must be an integer between 1 and 65535")
	}
	return Config{HTTPPort: strconv.Itoa(n)}, nil
}
