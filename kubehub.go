package main

import (
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	log "github.com/Sirupsen/logrus"
	"os"
	"strings"
)

func main() {
	log.SetOutput(os.Stderr)

	var options Options
	var err = options.Parse()
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			log.Errorf("Problem parsing options %v", err)
		}
		os.Exit(1)
	}

	level, err := log.ParseLevel(options.LogLevel)
	if err != nil {
		log.Errorf("Problem parsing error level %v", err)
		os.Exit(1)
	}
	log.SetLevel(level)
	log.Info("Starting kubehub")

	cfg := &client.Config{
		Host:        options.Kubernetes.Host,
		Username:    options.Kubernetes.Username,
		Password:    options.Kubernetes.Password,
		BearerToken: options.Kubernetes.Token,
	}
	client, _ := client.New(cfg)
	config := &Config{}

	process, err := NewProcess(client, config, options.File)
	if err != nil {
		log.Errorf("Problem creating process %v", err)
		os.Exit(1)
	}

	api, err := NewApi(process)
	if err != nil {
		log.Error("Problem creating api %v", err)
		os.Exit(1)
	}

	api.Serve(":8081")
}
