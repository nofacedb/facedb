package cfgparser

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/nofacedb/facedb/internal/version"
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
	flag.StringVar(&cfgName, "config", "", "path to yaml config file")
	printVersion := false
	flag.BoolVar(&printVersion, "version", false, "print FACEDB version")
	flag.Parse()

	if printVersion {
		fmt.Println(version.Version)
		os.Exit(0)
	}

	return &CArgs{
		CFGName: cfgName,
	}
}

// HTTPServerCFG contains config for HTTP Server.
type HTTPServerCFG struct {
	Addr           string `yaml:"addr"`
	Port           int    `yaml:"port"`
	WriteTimeoutMS int    `yaml:"write_timeout_ms"`
	ReadTimeoutMS  int    `yaml:"read_timeout_ms"`
	KeyPath        string `yaml:"key_path"`
	CrtPath        string `yaml:"crt_path"`
}

// HTTPClientCFG contains config for HTTP Client.
type HTTPClientCFG struct {
	TimeoutMS int `yaml:"timeout_ms"`
}

// StorageCFG contains config for facial features storage.
type StorageCFG struct {
	Addr           string  `yaml:"addr"`
	Port           int     `yaml:"port"`
	User           string  `yaml:"user"`
	Password       string  `yaml:"passwd"`
	MaxPings       int     `yaml:"max_pings"`
	DefaultDB      string  `yaml:"default_db"`
	WriteTimeoutMS int     `yaml:"write_timeout_ms"`
	ReadTimeoutMS  int     `yaml:"read_timeout_ms"`
	ImgPath        string  `yaml:"img_path"`
	Debug          bool    `yaml:"debug"`
	CosineBoundary float64 `yaml:"cosine_boundary"`
}

// FaceRecognizersCFG contains config for face recognition engine.
type FaceRecognizersCFG struct {
	FaceRecognizers []string `yaml:"face_recognizers"`
	AwImgsQMaxSize  int      `yaml:"aw_imgs_q_max_size"`
	AwImgsQCleanMS  int      `yaml:"aw_imgs_q_clean_ms"`
}

// ControlPanelsCFG contains config for control panels.
type ControlPanelsCFG struct {
	ControlPanels []string `yaml:"control_panels"`
	ACOQMaxSize   int      `yaml:"aco_q_max_size"`
	ACOQCleanMS   int      `yaml:"aco_q_clean_ms"`
	ACQMaxSize    int      `yaml:"ac_q_max_size"`
	ACQCleanMS    int      `yaml:"ac_q_clean_ms"`
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
	HTTPClientCFG      HTTPClientCFG      `yaml:"h-ttp_client"`
	StorageCFG         StorageCFG         `yaml:"storage"`
	FaceRecognizersCFG FaceRecognizersCFG `yaml:"face_recognizers"`
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

	if cfg.StorageCFG.ImgPath[len(cfg.StorageCFG.ImgPath)-1] == '/' {
		cfg.StorageCFG.ImgPath = cfg.StorageCFG.ImgPath[:len(cfg.StorageCFG.ImgPath)-1]
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
