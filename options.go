package main

import (
	"github.com/jessevdk/go-flags"
)

type KubernetesOptions struct {
	Host     string `toml:"nodes" short:"h" long:"host" description:"Kubernetes host"`
	Username string `toml:"username" long:"user" description:"Username"`
	Password string `toml:"password" long:"pass" description:"Password"`
	token    string `toml:"token" long:"token" description:"Bearer token"`
	CaKey    string `toml:"cakey" long:"cakey" description:"CA key"`
	Cert     string `toml:"cert" long:"cert" description:"Certificate"`
	Key      string `toml:"key" long:"key" description:"Key"`
}

type Options struct {
	Kubernetes KubernetesOptions `toml:"kubernetes" group:"Kubernetes Options" namespace:"kube"`
	LogLevel   string            `toml:"log_level" short:"v" long:"log_level" description:"Loglevel"`
	File       string            `short:"f" long:"file" description:"A file" value-name:"FILE"`
}

func (o *Options) Parse() error {
	var parser = flags.NewParser(o, flags.Default)

	_, err := parser.Parse()
	return err
}
