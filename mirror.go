package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	log "github.com/Sirupsen/logrus"
	"github.com/cavaliercoder/grab"
	humanize "github.com/dustin/go-humanize"
	"github.com/mgutz/ansi"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
)

type archive struct {
	archiveType string
	Size        uint64
	Checksum    string
	URL         string
	Version     decimal.Decimal
	request     *grab.Request
}

const urlRepo11 = "http://dl.google.com/android/repository/repository-11.xml"
const urlAddonsList2 = "http://dl.google.com/android/repository/addons_list-2.xml"
const urlAddon = "http://dl.google.com/android/repository/addon.xml"

//Process is entry point for processing a repository url
//takes the url and the output directory path to save assets
func Process(url string, outputDir string, silent bool) {
	var archives []*archive
	if url == "" {
		archives = processRepo(urlRepo11, outputDir, silent)
		writeFile(urlAddonsList2, outputDir)
		archives = append(archives, processRepo(urlAddon, outputDir, silent)...)
	} else {
		archives = processRepo(url, outputDir, silent)
	}
	downloadArchives(archives, outputDir, silent)
}

func processRepo(url string, outputDir string, silent bool) []*archive {
	fmt.Printf(ansi.Color(fmt.Sprintf("Start processing for url: %s", url), "blue+b:white"))
	fmt.Printf("\nAssets will be downloaded to: %s\n", outputDir)
	repoXML, err := fetchFile(url)
	if err != nil {
		log.Error("Unable to fetch repo XML", err)
		return nil
	}
	writeFile(url, outputDir)
	repoXML = sanitizeXML(repoXML)
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(repoXML))
	if err != nil {
		log.Error("Unable to parse repo XML", err)
		return nil
	}
	//html, _ := doc.Html()
	//fmt.Printf("%s\n", html)

	return getArchives(doc)
}

func sanitizeXML(repoXML string) string {
	repoXML = strings.Replace(repoXML, "sdk:", "", -1)
	repoXML = strings.Replace(repoXML, "<obsolete/>", "<obsolete>true</obsolete>", -1)
	repoXML = strings.Replace(repoXML, "source>", "sdk-source>", -1)
	reg, err := regexp.Compile("<[^<]*?/>")
	if err != nil {
		log.Fatal(err)
	}
	repoXML = reg.ReplaceAllString(repoXML, "")
	return repoXML
}

func getArchives(doc *goquery.Document) []*archive {
	var archives []*archive

	archiveTopVersion := map[string]decimal.Decimal{}
	//get all relevant data from xml into archive struct
	doc.Find("archives").Each(func(i int, archiveNode *goquery.Selection) {
		//fmt.Printf("%s\n", goquery.NodeName(archiveNode))

		archiveTypeNode := getArchiveTypeNode(archiveNode)
		archiveTypeName := goquery.NodeName(archiveTypeNode)
		if archiveTypeName == "add-on" {
			archiveTypeName = archiveTypeNode.Find("name-id").Text()
		} else if archiveTypeName == "extra" {
			archiveTypeName = archiveTypeNode.Find("path").Text()
		}

		apiLevel := getAPIVersion(archiveTypeNode)
		if apiLevel.Cmp(decimal.NewFromFloat(0)) == 0 {
			revision := archiveTypeNode.Find("revision>major").Text() + "." +
				archiveTypeNode.Find("revision>minor").Text() +
				archiveTypeNode.Find("revision>micro").Text()
			//fmt.Printf("%s\n", revision)
			apiLevel, _ = decimal.NewFromString(revision)
		}

		apiLevelOld := archiveTopVersion[archiveTypeName]
		if apiLevelOld.Cmp(apiLevel) == -1 {
			archiveTopVersion[archiveTypeName] = apiLevel
			//fmt.Printf("%s: new max api:%s, oldAPI: %s\n", archiveTypeName, apiLevel.String(), apiLevelOld.String())
		}

		archiveNode.Find("archive").Each(func(i int, node *goquery.Selection) {
			checksum := node.Find("checksum").Text()
			url := node.Find("url").Text()
			size, _ := strconv.ParseUint(node.Find("size").Text(), 10, 64)
			archives = append(archives, &archive{archiveTypeName, size, checksum, url, apiLevel, nil})
			//fmt.Printf("%s, \t\t%d\n", archiveTypeName, size)
		})
	})

	//filter out the archive structs that we dont want
	var filteredArchives []*archive
	for _, value := range archives {
		if archiveTopVersion[value.archiveType].Cmp(value.Version) == 0 {
			filteredArchives = append(filteredArchives, value)
		}
	}

	for i, value := range filteredArchives {
		log.Debug(fmt.Sprintf("%d: %v", i, value))
	}
	return filteredArchives
}

func getArchiveTypeNode(archiveNode *goquery.Selection) *goquery.Selection {
	parent := archiveNode.Parent()
	return parent
}

func getAPIVersion(node *goquery.Selection) decimal.Decimal {
	var level decimal.Decimal
	if node.Has("api-level").Length() > 0 {
		level, _ = decimal.NewFromString(node.Find("api-level").Text())
	}
	return level
}

func shouldDownload(parent *goquery.Selection) bool {
	nodesToFilterOut := []string{"doc", "sdk-source"}
	parentName := goquery.NodeName(parent)
	if !containsString(nodesToFilterOut, parentName) {
		fmt.Printf("will dld parent: %v\turl: %v\n", parentName, parent.Find("url").Text())
		// siblings.Each(func(i int, node *goquery.Selection) {
		// 	fmt.Printf("node: %v\n", goquery.NodeName(node))
		// })
	}
	return false
}

func getTotalSize(archives []*archive, outputDir string) (uint64, int) {
	var totalSize uint64
	var num int
	for _, value := range archives {
		fullname := outputDir + string(os.PathSeparator) + value.URL
		if _, err := os.Stat(fullname); err != nil {
			totalSize += value.Size
			num++
		}
	}
	return totalSize, num
}

func downloadArchives(archives []*archive, outputDir string, silent bool) {
	if !silent {
		totalSize, num := getTotalSize(archives, outputDir)
		msgFiles := ansi.Color(fmt.Sprintf("%d file(s) of total size: %v", num, humanize.Bytes(totalSize)), "red+b:white")
		fmt.Printf("Are you sure you want to download %s (yes/no): ", msgFiles)
		goAhead := askForConfirmation()
		if !goAhead {
			return
		}
	}
	var reqs []*grab.Request
	for _, file := range archives {
		log.Debug(fmt.Sprintf("Got url: %s\n", getFileURL(file.URL)))
		req, err := grab.NewRequest(getFileURL(file.URL))
		if err != nil {
			log.Error(fmt.Sprintf("Error downloading:%s\n%v\n", file.URL, err))
		}
		file.request = req
		hexCheckSum, _ := hex.DecodeString(file.Checksum)
		req.SetChecksum("sha1", hexCheckSum)
		req.Filename = outputDir + string(os.PathSeparator) + file.URL
		reqs = append(reqs, req)
	}

	client := grab.NewClient()
	respch := client.DoBatch(3, reqs...)

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
						fmt.Fprintf(os.Stderr, "Error downloading %s: %v\n", resp.Request.URL(),
							resp.Error)
					} else {
						fmt.Printf("Finished %s %d / %d bytes (%d%%)\n", resp.Filename,
							resp.BytesTransferred(), resp.Size, int(100*resp.Progress()))
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
					fmt.Printf("Downloading %s %d / %d bytes (%d%%)\033[K\n", resp.Filename,
						resp.BytesTransferred(), resp.Size, int(100*resp.Progress()))
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
