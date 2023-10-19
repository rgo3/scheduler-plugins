package interferenceaware

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type InterferenceData struct {
	CPU   float64 `json:"cpu"`
	LLC   float64 `json:"llc"`
	Mem   float64 `json:"mem"`
	Blk   float64 `json:"blk"`
	NetPR float64 `json:"netpr"`
	NetBW float64 `json:"netbw"`
}

func sanitizeTaskName(taskName string) string {
	key := strings.ReplaceAll(taskName, " ", "")
	key = strings.ReplaceAll(key, ":", ".")
	key = strings.ReplaceAll(key, "(", ".")
	key = strings.ReplaceAll(key, ")", ".")

	return key
}

func populateInterferenceMetrics(dirPath string, resource string) (map[string]float64, error) {
	dataMap := make(map[string]float64)

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("reading directory: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() || e.Name() == "..data" {
			continue // Skip directories
		}

		filePath := filepath.Join(dirPath, e.Name())
		data, err := readInterferenceData(filePath)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", e.Name(), err)
		}

		// Store the data in the map with the filename as the key
		switch resource {
		case "cpu":
			dataMap[e.Name()] = data.CPU
		case "bio":
			dataMap[e.Name()] = data.Blk
		default:
			return nil, fmt.Errorf("unsupported resource %s", resource)
		}
	}

	return dataMap, nil
}

func readInterferenceData(filename string) (InterferenceData, error) {
	fileContent, err := os.ReadFile(filename)
	if err != nil {
		return InterferenceData{}, err
	}

	var data InterferenceData
	if err := json.Unmarshal(fileContent, &data); err != nil {
		return InterferenceData{}, err
	}

	return data, nil
}
