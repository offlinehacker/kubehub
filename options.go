package main

import (
	"github.com/jessevdk/go-flags"
)

type KubernetesOptions struct {
	Host     string `long:"host" description:"Kubernetes host" default:"http://localhost:8080"`
	Username string `long:"user" description:"Kubernetes username" default:""`
	Password string `long:"pass" description:"Kubernetes password" default:""`
	Token    string `long:"token" description:"Bearer token" default:""`
	//CaKey    string `toml:"cakey" long:"cakey" description:"CA key"`
	//Cert     string `toml:"cert" long:"cert" description:"Certificate"`
	//Key      string `toml:"key" long:"key" description:"Key"`
}

type Options struct {
	Kubernetes KubernetesOptions `group:"Kubernetes Options" namespace:"kube"`
	LogLevel   string            `short:"v" long:"log_level" description:"Loglevel panic/fatal/error/warn/info/debug" default:"info"`
	File       string            `short:"c" long:"config" description:"Config file" value-name:"FILE"`
	Host       string            `short:"h" long:"host" description:"Host where to serve" value-name:"HOST" default:":8081"`
}

func (o *Options) Parse() error {
	var parser = flags.NewParser(o, flags.Default)

	_, err := parser.Parse()
	return err
}
