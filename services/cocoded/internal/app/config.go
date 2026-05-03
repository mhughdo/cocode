package app

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
)

type Config struct {
	Addr             string
	AuthToken        string
	LogPath          string
	DataDir          string
	DBPath           string
	ArtifactDir      string
	GitHubAPIBaseURL string
	Version          string
}

func LoadConfig() (Config, error) {
	dataDir := getenv("COCODED_DATA_DIR", defaultDataDir())
	token := os.Getenv("COCODED_AUTH_TOKEN")
	if token == "" {
		generated, err := GenerateAuthToken()
		if err != nil {
			return Config{}, err
		}
		token = generated
	}

	return Config{
		Addr:             getenv("COCODED_ADDR", "127.0.0.1:17658"),
		AuthToken:        token,
		LogPath:          os.Getenv("COCODED_LOG_PATH"),
		DataDir:          dataDir,
		DBPath:           getenv("COCODED_DB_PATH", filepath.Join(dataDir, "cocoded.sqlite")),
		ArtifactDir:      getenv("COCODED_ARTIFACT_DIR", filepath.Join(dataDir, "artifacts")),
		GitHubAPIBaseURL: os.Getenv("COCODED_GITHUB_API_BASE_URL"),
		Version:          Version,
	}, nil
}

func GenerateAuthToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".cocode"
	}
	return filepath.Join(home, ".cocode")
}
