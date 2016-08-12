package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/pkg/errors"
)

//Process is entry point for processing a repository url
//takes the url and the output directory path to save assets
func Process(url string, outputDir string) {
	fmt.Printf("Start processing for url: %s\n", url)
	fmt.Printf("Assets will be downloaded to: %s\n", outputDir)
	_, err := fetchFile(url)
	if err != nil {
		panic(err)
	}
	writeFile(url, outputDir)
}

func fetchFile(url string) (string, error) {
	fileName := getFileName(url)
	fmt.Printf("Reading file: %s\n", fileName)
	response, err := http.Get(url)
	if err != nil {
		return "", errors.Wrap(err, "ERR!: Unable to download file")
	}
	defer response.Body.Close()
	bodyBytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", errors.Wrap(err, "ERR!: Unable to read response")
	}
	body := string(bodyBytes)
	return body, nil
}

func writeFile(url string, outputDir string) error {
	fileName := getFileName(url)
	fmt.Printf("Downloading file: %s\n", fileName)
	response, err := http.Get(url)
	if err != nil {
		return errors.Wrap(err, "ERR!: Unable to download file")
	}
	defer response.Body.Close()
	if err != nil {
		return errors.Wrap(err, "ERR!: Unable to read response")
	}
	out, err := os.Create(outputDir + string(os.PathSeparator) + fileName)
	if err != nil {
		return errors.Wrap(err, "ERR!: Unable to create file")
	}
	defer out.Close()
	io.Copy(out, response.Body)
	return nil
}

func getFileName(url string) string {
	lastSlash := strings.LastIndex(url, "/") + 1
	return url[lastSlash:]
}
