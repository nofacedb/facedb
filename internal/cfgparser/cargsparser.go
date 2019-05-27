package cfgparser

import (
	"flag"
	"fmt"
	"os"

	"github.com/nofacedb/facedb/internal/version"
)

type cArgs struct {
	ConfigPath string
	Addr       string
	Port       int
}

func parseCArgs() *cArgs {
	cargs := &cArgs{}
	pv := false
	flag.StringVar(&(cargs.ConfigPath), "config", "", "path to YAML configuration file")
	flag.StringVar(&(cargs.Addr), "addr", "", "server address (if specified, overrides address from configuration file)")
	flag.IntVar(&(cargs.Port), "port", -1, "server port (if specified, overrides port from configuration file)")
	flag.BoolVar(&pv, "version", false, "print server version")
	flag.Parse()

	if pv {
		fmt.Println(version.Version)
		os.Exit(0)
	}

	return cargs
}
