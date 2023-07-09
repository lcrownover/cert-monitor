package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Domains []string `yaml:"domains,omitempty"`
}

func main() {
	var (
		defaultConfigFilePath = "/etc/cert-monitor/config.yml"
		config                Config
		err                   error
	)
	configFilePath, found := os.LookupEnv("CERT_MONITOR_CONFIG_PATH")
	if !found {
		configFilePath = defaultConfigFilePath
	}
	d, err := os.ReadFile(configFilePath)
	if err != nil {
		os.Stderr.WriteString(fmt.Sprintf("failed to read config file: %s\n", configFilePath))
		os.Exit(1)
	}
	err = yaml.Unmarshal(d, &config)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%v\n", config)
}
