package utils

import (
	"encoding/csv"
	"log"
	"os"
	"strings"
)

func WriteToCsv(filename string, header []string, records [][]string) (string, error) {
	if !strings.HasSuffix(filename, ".csv") {
		filename = filename + ".csv"
	}
	file, err := os.Create(filename)
	if err != nil {
		return "", err
	}

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.Write(header); err != nil {
		return "", err
	}

	if err := writer.WriteAll(records); err != nil {
		return "", err
	}

	fcErr := file.Close()
	if fcErr != nil {
		return "", fcErr
	}
	log.Printf("Data successfully written to %s", filename)
	return filename, nil
}
