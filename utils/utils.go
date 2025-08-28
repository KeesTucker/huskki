package utils

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
)

func RoundToXDp(f float64, dp uint8) float64 {
	e := math.Pow(10, float64(dp))
	return math.Round(f*e) / e
}

func BoolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
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
