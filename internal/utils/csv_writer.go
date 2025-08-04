package utils

import (
	"encoding/csv"
	"log"
	"os"
	"strings"
)

func WriteToCsv(filename string, header []string, records [][]string) error {
	if !strings.HasSuffix(filename, ".csv") {
		filename = filename + ".csv"
	}
	file, fErr := os.Create(filename)
	if fErr != nil {
		return fErr
	}

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if whErr := writer.Write(header); whErr != nil {
		return whErr
	}

	if wrErr := writer.WriteAll(records); wrErr != nil {
		return wrErr
	}

	fcErr := file.Close()
	if fcErr != nil {
		return fcErr
	}
	log.Println("Data successfully written to output.csv")
	return nil
}
