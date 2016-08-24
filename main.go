//android-sdk-mirror lets you create a local mirror of an android sdk repository.
//This can be very helpful in environments with restricted internet access.
package main

import (
	"fmt"
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/mandarl/go-selfupdate/selfupdate"
	"github.com/mkideal/cli"
)

var VERSION string = "dev"

type argT struct {
	cli.Helper
	Url       string `cli:"*u,url" usage:"url of the repository you want to mirror"`
	OutputDir string `cli:"o,output-dir" usage:"output directory to save downloaded assets" dft:"."`
	Version   bool   `cli:"!v,version" usage:"print the current version"`
	Verbose   bool   `cli:"verbose" usage:"enable verbose logging"`
	Silent    bool   `cli:"q,silent" usage:"suppresses any user input prompts"`
}

func main() {
	cli.Run(&argT{}, func(ctx *cli.Context) error {
		argv := ctx.Argv().(*argT)
		run(argv)
		return nil
	})
}

func run(args *argT) {

	if args.Version {
		fmt.Printf("android-sdk-mirror: Verison: %s\n", VERSION)
		os.Exit(0)
	}
	if args.Verbose {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.ErrorLevel)
	}

	runUpdate()
	Process(args.Url, args.OutputDir, args.Silent)
}

func runUpdate() {
	var updater = &selfupdate.Updater{
		CurrentVersion: VERSION,
		ApiURL:         "http://dipoletech.com/projects/dist/",
		//u.fetch(u.ApiURL + u.CmdName + "/" + plat + ".json")
		BinURL: "http://dipoletech.com/projects/dist/",
		//u.BinURL + u.CmdName + "/" + u.Info.Version
		//  + "/" + plat + ".gz"
		DiffURL: "",
		//u.fetch(u.DiffURL + u.CmdName + "/" + u.CurrentVersion
		//  + "/" + u.Info.Version + "/" + plat)
		Dir:     "update/",
		CmdName: "android-sdk-mirror", // this is added to apiurl to get json
	}

	if updater != nil {
		go updater.BackgroundRun()
	}
}
