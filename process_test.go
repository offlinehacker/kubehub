package main

import (
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	log "github.com/Sirupsen/logrus"
	"os"
	"testing"
)

func TestProcessCreateResource(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	log.SetOutput(os.Stderr)

	cfg := &client.Config{
		Host: "http://localhost:8080",
	}
	client, _ := client.New(cfg)
	config := &Config{}
	config.Project = "test"

	p := NewProcess(client, config)

	f, _ := os.Open("test.yaml")
	config.Load(f)
	p.CreateNamespaces()
}
