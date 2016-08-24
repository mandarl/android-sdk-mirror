package main

import (
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"math/big"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/cavaliercoder/grab"
	"github.com/pkg/errors"
)

type archive struct {
	archiveType string
	Size        int    `xml:"size"`
	Checksum    string `xml:"checksum"`
	URL         string `xml:"url"`
	request     *grab.Request
}

type repo struct {
	Items []struct {
		XMLName  xml.Name
		RepoType struct {
			APILevel int    `xml:"revision"`
			Desc     string `xml:"description"`
			Obsolete bool   `xml:"obsolete"`
			Archives struct {
				ArchiveItem []struct {
					Size     int    `xml:"size"`
					Checksum string `xml:"checksum"`
					URL      string `xml:"url"`
				} `xml:"archive"`
			} `xml:"archives"`
		}
	} `xml:",any"`
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
	repoXML = sanitizeXML(repoXML)

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(repoXML))
	if err != nil {
		return errors.Wrap(err, "Unable to parse repo XML")
	}
	//html, _ := doc.Html()
	//fmt.Printf("%s\n", html)
	getArchives(doc)
	//downloadArchives(archives)
	return nil
}

func sanitizeXML(repoXML string) string {
	repoXML = strings.Replace(repoXML, "sdk:", "", -1)
	repoXML = strings.Replace(repoXML, "<obsolete/>", "<obsolete>true</obsolete>", -1)
	repoXML = strings.Replace(repoXML, "source>", "sdk-source>", -1) //"<obsolete>true</obsolete>", -1)
	reg, err := regexp.Compile("<[^<]*?/>")
	if err != nil {
		log.Fatal(err)
	}
	repoXML = reg.ReplaceAllString(repoXML, "")
	return repoXML
}

func getArchives(doc *goquery.Document) []*archive {
	var archives []*archive

	//var archiveTypes map[string][]archive
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
		if apiLevel == 0 {
			revision := archiveTypeNode.Find("revision>major").Text() + "." +
				archiveTypeNode.Find("revision>minor").Text() +
				archiveTypeNode.Find("revision>micro").Text()
			fmt.Printf("%s\n", revision)
			apiLevel, _ = strconv.Atoi(revision)
		}

		fmt.Printf("%s, \t\t%d\n", archiveTypeName, apiLevel)

		//if shouldDownload(archiveTypeNode) {
		//nodes.Find("archive").Each(func(i int, node *goquery.Selection) {
		// checksum := node.Find("checksum").Text()
		// url := node.Find("url").Text()
		// archives = append(archives, &archive{checksum, url, nil})
		//})
		//}
	})

	//filter out the archive structs that we dont want
	return archives
}

func getArchiveTypeNode(archiveNode *goquery.Selection) *goquery.Selection {
	parent := archiveNode.Parent()
	return parent
}

func getAPIVersion(node *goquery.Selection) big.Rat {
	level := big.NewRat(0, math.MaxInt64)
	outerHTML, _ := goquery.OuterHtml(node)
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(outerHTML))
	if doc.Has("api-level").Length() > 0 {
		//fmt.Printf("%s\n", outerHTML)
		level, _ = big.NewRat(doc.Find("api-level").First().Text())
	}
	return level
}

func shouldDownload(parent *goquery.Selection) bool {
	nodesToFilterOut := []string{"doc", "sdk-source"}
	parentName := goquery.NodeName(parent)
	if !stringInSlice(parentName, nodesToFilterOut) {
		fmt.Printf("will dld parent: %v\turl: %v\n", parentName, parent.Find("url").Text())
		// siblings.Each(func(i int, node *goquery.Selection) {
		// 	fmt.Printf("node: %v\n", goquery.NodeName(node))
		// })
	}
	return false
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func downloadArchives(archives []*archive) {
	var reqs []*grab.Request
	for _, file := range archives {
		fmt.Printf("%s\n", getFileURL(file.URL))
		continue
		req, err := grab.NewRequest(getFileURL(file.URL))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error downloading:%s\n%v\n", file.URL, err)
		}
		//file.request = req
		hexCheckSum, _ := hex.DecodeString(file.Checksum)
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
