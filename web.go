package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
)

func startWebServer(outputDir string, port int) {
	http.Handle("/android/repository/", http.StripPrefix("/android/repository/", http.FileServer(http.Dir(outputDir))))
	fmt.Printf("Serving files from %s\n", outputDir)
	fmt.Printf("Add the following URLs to SDK Manager:\n")
	localIP := getLocalIP()
	fmt.Printf("%s\n", strings.Replace(urlRepo11, "dl.google.com", localIP, 1))
	fmt.Printf("%s\n", strings.Replace(urlAddonsList2, "dl.google.com", localIP, 1))
	fmt.Printf("%s\n", strings.Replace(urlAddon, "dl.google.com", localIP, 1))

	err := http.ListenAndServe(":"+strconv.Itoa(port), nil) //localIP+":"+string(port), nil)
	if err != nil {
		panic(err)
		log.Fatalf("Could not start webserver on port: %d", port)

	}

}

// getLocalIP returns the non loopback local IP of the host
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback the display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}
