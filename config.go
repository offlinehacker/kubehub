package main

import (
	"bytes"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/runtime"
	"gopkg.in/yaml.v2"
	"io"
	"text/template"
)

type Config struct {
	// Name of the project
	Project string `json:"project" yaml:"project"`

	// List of all avalible applications
	Applications []Application `json:"applications" yaml:"applications"`

	// List of all avalibele application groups
	ApplicationGroups []ApplicationGroup `json:"groups" yaml:"groups"`

	// List of all avalible templates
	Templates []Template `json:"templates" yaml:"templates"`

	// List of all avalible namespaces
	Namespaces []Namespace `json:"namespaces" yaml:"namespaces"`
}

// Writes config to a file
func (c *Config) Commit(writer io.Writer) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	if _, err := writer.Write(data); err != nil {
		return err
	}

	return nil
}

// Loads config from a file
func (c *Config) Load(reader io.Reader) error {
	buf := bytes.NewBuffer(nil)
	io.Copy(buf, reader)

	if err := yaml.Unmarshal(buf.Bytes(), c); err != nil {
		return err
	}

	return nil
}

type Template struct {
	// Template name
	Name string `json:"name" yaml:"name" description:"Template name"`

	// Template content
	Content string `json:"template" yaml:"template" description:"YAML or JSON formated kubernetes config template"`
}

// Generates config from template
func (t *Template) Generate(client *client.Client, data map[string]string) (runtime.Object, string, error) {
	buf := new(bytes.Buffer)

	tp, err := template.New("tpl").Parse(t.Content)
	if err != nil {
		return nil, "", err
	}

	if tp.Execute(buf, data) != nil {
		return nil, "", err
	}

	obj, err := runtime.YAMLDecoder(client.Codec).Decode([]byte(buf.String()))
	if err != nil {
		return nil, "", err
	}

	_, objKind, err := api.Scheme.ObjectVersionAndKind(obj)

	return obj, objKind, err
}

type Application struct {
	// Application name
	Name string `json:"name" yaml:"name" description:"Name of the application"`

	// Replication controller name used by application
	ReplicationController string `json:"replicationController" yaml:"replicationController" description:"Name of the repplication controller template"`

	// Service template name used by application
	Service string `json:"service" yaml:"service" description:"Name of the service template"`

	// Application tags
	Tags map[string]string `json:"tags" yaml:"tags" description:"Template tags associated with application"`
}

// Groups of applications
type ApplicationGroup struct {
	// Application group name
	Name string `json:"name" yaml:"name" description:"Name of the application group"`

	// List of all applications in a group
	Applications []string `json:"apps" yaml:"apps" description:"List of application names in group"`

	// Application group tags
	Tags map[string]string `json:"tags" yaml:"tags" description:"Template tags associated with application group"`
}

type Namespace struct {
	// Namespace name
	Name string `json:"name" yaml:"name" description:"Name of the namespace"`

	// Selected group of applications for namespace
	ApplicationGroup string `json:"group" yaml:"group" description:"Name of the application group associated with namespace"`

	// Namespace tags
	Tags map[string]string `json:"tags" yaml:"tags" description:"Template tags associated with namespace"`
}
