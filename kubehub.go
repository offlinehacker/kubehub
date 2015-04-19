package main

import (
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	log "github.com/Sirupsen/logrus"
	"os"
)

func main() {
	log.SetLevel(log.DebugLevel)
	log.SetOutput(os.Stderr)
	log.Debug("Starting kubehub")

	var options Options
	var err = options.Parse()

	if err != nil {
		log.Error("Problem parsing options %v", err)
		os.Exit(1)
	}

	cfg := &client.Config{
		Host: "http://localhost:8080",
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
