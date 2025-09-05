package main

import (
	"bufio"
	"log"
	"os"
	"strings"
)

const (
	sniffLocation = "logs/read.txt"
	outLocation   = "logs/read_filtered.txt"
)

func main() {
	file, err := os.Open(sniffLocation)
	if err != nil {
		log.Fatal(err)
		return
	}
	defer file.Close()

	outFile, err := os.Create(outLocation)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if err = scanner.Err(); err != nil {
			log.Fatal(err)
		}
		if strings.Contains(line, "]  07 23") {
			_, err = outFile.WriteString(line + "\n")
			if err != nil {
				log.Fatal(err)
				return
			}
		}
	}

	// Flush to disk
	err = outFile.Sync()
	if err != nil {
		log.Fatal(err)
		return
	}
}
