package config

import (
	"os"
	"path/filepath"
	"runtime"
)

var (
	_, b, _, _ = runtime.Caller(0)

	// Root folder of this project
	rootPath = filepath.Join(filepath.Dir(b), "./../..")
)

func configFilePath() string {
	if os.Getenv("APP_CONFIG_PATH") != "" {
		return os.Getenv("APP_CONFIG_PATH")
	}
	return rootPath
}
