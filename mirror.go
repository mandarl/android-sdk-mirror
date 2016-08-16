package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/cavaliercoder/grab"
	"github.com/pkg/errors"
)

type archive struct {
	checksum string
	URL      string
	request  *grab.Request
}

//Process is entry point for processing a repository url
//takes the url and the output directory path to save assets
func Process(url string, outputDir string) {
	fmt.Printf("Start processing for url: %s\n", url)
	fmt.Printf("Assets will be downloaded to: %s\n", outputDir)
	repoXML, err := fetchFile(url)
	if err != nil {
		panic(err)
	}
	writeFile(url, outputDir)
	err = processRepo(repoXML)
	if err != nil {
		panic(err)
	}
}

func processRepo(repoXML string) error {
	repoXML = strings.Replace(repoXML, "sdk:", "", -1)
	//repoXML = strings.Replace(repoXML, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>", "", -1)
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(repoXML))
	if err != nil {
		return errors.Wrap(err, "Unable to parse repo XML")
	}
	var archives []*archive
	doc.Find("archives").Each(func(i int, nodes *goquery.Selection) {
		nodes.Find("archive").Each(func(i int, node *goquery.Selection) {
			checksum := node.Find("checksum").Text()
			url := node.Find("url").Text()
			archives = append(archives, &archive{checksum, url, nil})
		})
	})
	downloadArchives(archives)
	return nil
}

func downloadArchives(archives []*archive) {
	var reqs []*grab.Request
	for _, file := range archives {
		req, err := grab.NewRequest(getFileURL(file.URL))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error downloading:%s\n%v\n", file.URL, err)
		}
		file.request = req
		hexCheckSum, _ := hex.DecodeString(file.checksum)
		req.SetChecksum("sha1", hexCheckSum)
		req.Filename = file.URL
		reqs = append(reqs, req)
	}

	client := grab.NewClient()
	respch := client.DoBatch(2, reqs...)

	t := time.NewTicker(200 * time.Millisecond)

	// monitor downloads
	completed := 0
	inProgress := 0
	var responses []*grab.Response
	for completed < len(reqs) {
		select {
		case resp := <-respch:
			// a new response has been received and has started downloading
			// (nil is received once, when the channel is closed by grab)
			if resp != nil {
				responses = append(responses, resp)
			}

		case <-t.C:
			// clear lines
			if inProgress > 0 {
				fmt.Printf("\033[%dA\033[K", inProgress)
			}

			// update completed downloads
			for i, resp := range responses {
				if resp != nil && resp.IsComplete() {
					// print final result
					if resp.Error != nil {
						fmt.Fprintf(os.Stderr, "Error downloading %s: %v\n", resp.Request.URL(), resp.Error)
					} else {
						fmt.Printf("Finished %s %d / %d bytes (%d%%)\n", resp.Filename, resp.BytesTransferred(), resp.Size, int(100*resp.Progress()))
					}

					// mark completed
					responses[i] = nil
					completed++
				}
			}

			// update downloads in progress
			inProgress = 0
			for _, resp := range responses {
				if resp != nil {
					inProgress++
					fmt.Printf("Downloading %s %d / %d bytes (%d%%)\033[K\n", resp.Filename, resp.BytesTransferred(), resp.Size, int(100*resp.Progress()))
				}
			}
		}
	}

	t.Stop()

	fmt.Printf("%d files successfully downloaded.\n", len(reqs))
}

func getFileURL(file string) string {
	return "http://dl.google.com/android/repository/" + file
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
