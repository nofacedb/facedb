package main

import (
	"fmt"
	"os"

	"github.com/nofacedb/facedb/internal/cfgparser"
	"github.com/nofacedb/facedb/internal/httpserver"
)

func main() {
	cargs := cfgparser.ParseCArgs()
	cfg, err := cfgparser.ReadCFG(cargs.CFGName)
	if err != nil {
		// Not logger, because it wasn't configured.
		fmt.Println(err)
		os.Exit(1)
	}

	serv, err := httpserver.CreateHTTPServer(cfg)
	if err != nil {
		// TODO: logger.
		fmt.Println(err)
		os.Exit(1)
	}

	serv.Run()
}
