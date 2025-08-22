package utils

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
)

func RoundTo1Dp(f float64) float64 {
	return math.Round(f*10) / 10
}

func RoundTo2Dp(f float64) float64 {
	return math.Round(f*100) / 100
}

func NextAvailableFilename(dir, name, ext string) string {
	path := filepath.Join(dir, name+ext)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}

	for i := 1; ; i++ {
		newName := fmt.Sprintf("%s_%d%s", name, i, ext)
		newPath := filepath.Join(dir, newName)
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return newPath
		}
	}
}
