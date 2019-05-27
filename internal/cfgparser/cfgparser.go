package cfgparser

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

// CArgs contains command line arguments.
type CArgs struct {
	CFGName string
}

// ParseCArgs parser command line arguments and returns them as *CArgs.
func ParseCArgs() *CArgs {
	cfgName := ""
	flag.StringVar(&cfgName, "config", "config.yaml", "path to yaml config file")
	printVersion := false
	flag.BoolVar(&printVersion, "version", false, "print FACEDB version")
	flag.Parse()

	if printVersion {
		// TODO: versioning based on MakeFiles.
		fmt.Println("FACEDB V0.0.1")
		os.Exit(0)
	}

	return &CArgs{
		CFGName: cfgName,
	}
}

// HTTPServerCFG contains config for HTTP Server.
type HTTPServerCFG struct {
	Name           string `yaml:"name"`
	Socket         string `yaml:"socket"`
	WriteTimeoutMS int    `yaml:"write_timeout_ms"`
	ReadTimeoutMS  int    `yaml:"read_timeout_ms"`
	ImmedResp      bool   `yaml:"immed_resp"`
	KeyPath        string `yaml:"key_path"`
	CrtPath        string `yaml:"crt_path"`
}

// FaceStorageCFG contains config for facial features storage.
type FaceStorageCFG struct {
	Addr           string  `yaml:"addr"`
	User           string  `yaml:"user"`
	Passwd         string  `yaml:"passwd"`
	NPing          int     `yaml:"n_ping"`
	DefaultDB      string  `yaml:"default_db"`
	WriteTimeoutMS int     `yaml:"write_timeout_ms"`
	ReadTimeoutMS  int     `yaml:"read_timeout_ms"`
	Debug          bool    `yaml:"debug"`
	SineBoundary   float64 `yaml:"sine_boundary"`
}

// FaceRecognitionCFG contains config for face recognition engine.
type FaceRecognitionCFG struct {
	FaceProcessors []string `yaml:"face_processors"`
	TimeoutMS      int      `yaml:"timeout_ms"`
}

// ControlPanelsCFG contains config for control panels.
type ControlPanelsCFG struct {
	ControlPanels []string `yaml:"control_panels"`
	TimeoutMS     int      `yaml:"timeout_ms"`
}

// LoggerCFG ...
type LoggerCFG struct {
	Output          string `yaml:"output"`
	UseColors       bool   `yaml:"use_colors"`
	UseTimestamp    bool   `yaml:"use_timestamp"`
	TimestampFormat string `yaml:"timestamp_format"`
	NonBlocking     bool   `yaml:"non_blocking"`
}

// CFG contains config for FACEDB server.
type CFG struct {
	HTTPServerCFG      HTTPServerCFG      `yaml:"http_server"`
	FaceRecognitionCFG FaceRecognitionCFG `yaml:"face_recognition"`
	FaceStorageCFG     FaceStorageCFG     `yaml:"face_storage"`
	ControlPanelsCFG   ControlPanelsCFG   `yaml:"control_panels"`
	LoggerCFG          LoggerCFG          `yaml:"logger"`
}

func readCFG(configPath string) (*CFG, error) {
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, errors.Wrap(err, "unable to read configuration file")
	}

	cfg := &CFG{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, errors.Wrap(err, "unable to parse configuration file")
	}

	return cfg, nil
}

// GetCFG ...
func GetCFG() (*CFG, error) {
	cargs := parseCArgs()
	if cargs.ConfigPath == "" {
		return nil, fmt.Errorf("path to YAML configuration file with carg \"--config\" is not specified")
	}

	cfg, err := readCFG(cargs.ConfigPath)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}
