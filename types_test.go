package main

import (
	"bytes"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"testing"
)

func TestTemplate(t *testing.T) {
	config := &client.Config{}
	client, err := client.New(config)

	content, err := ioutil.ReadFile("./test-service.yaml")
	tpl := Template{"test", string(content)}
	obj, kind, err := tpl.Generate(client, map[string]string{"name": "frontend"})

	if err != nil {
		t.Errorf("expected success, got %v", err)
	}

	if kind != "Service" {
		t.Errorf("expected Service kind, got %v", kind)
	}

	service := obj.(*api.Service)
	if service.Name != "frontend" {
		t.Errorf("expected name frontend, got %v", service.Name)
	}

	if service.Spec.Ports[0].Port != 80 {
		t.Errorf("expected name 80, got %v", service.Spec.Ports)
	}
}

func TestLoad(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	f, _ := os.Open("test.yaml")

	conf := Config{}
	if err := conf.Load(f); err != nil {
		t.Errorf("expected success, got %v", err)
	}

	conf.Commit(buf)
	if err := yaml.Unmarshal(buf.Bytes(), &conf); err != nil {
		t.Errorf("expected success, got %v", err)
	}

	if conf.Applications[0].Name != "guard" {
		t.Errorf("expected name 'guard', got %v", conf.Applications[0].Name)
	}
}
