package main

import (
	"bytes"
	"errors"
	"os"
	"sync"
	"time"
	//"fmt"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/fields"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/kubectl"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	log "github.com/Sirupsen/logrus"
	"github.com/imdario/mergo"
)

const (
	StateProcessing = iota
	StateReady      = iota
)

type Entity struct {
	Value     interface{}
	Processed bool
}

type BufferLogger struct {
	Entries []*log.Entry
}

func (l *BufferLogger) Fire(entry *log.Entry) error {
	l.Entries = append(l.Entries, entry)
	return nil
}

func (hook *BufferLogger) Levels() []log.Level {
	return []log.Level{
		log.PanicLevel,
		log.FatalLevel,
		log.ErrorLevel,
		log.WarnLevel,
		log.InfoLevel,
		log.DebugLevel,
	}
}

func NewBufferLoggerHook() *BufferLogger {
	return &BufferLogger{}
}

type Process struct {
	Kube    *client.Client
	Config  *Config
	mutex   sync.Mutex
	err     error
	logger  *BufferLogger
	state   int
	cfgFile string
}

func NewProcess(Kube *client.Client, Config *Config, cfgFile string) (*Process, error) {
	log.Infof("Loading config file %v", cfgFile)

	f, err := os.Open(cfgFile)
	if err != nil {
		return nil, err
	}

	if err := Config.Load(f); err != nil {
		return nil, err
	}

	f.Close()
	return &Process{Config: Config, Kube: Kube, state: StateReady, mutex: sync.Mutex{}, cfgFile: cfgFile}, nil
}

func (p *Process) Commit() error {
	log.Info("Deploying new config")

	p.mutex.Lock()

	f, err := os.Create(p.cfgFile)
	if err != nil {
		p.mutex.Unlock()
		return err
	}

	p.Config.Commit(f)
	f.Close()

	go func() {
		p.state = StateProcessing
		logger := log.New()
		logger.Level = log.DebugLevel
		p.logger = NewBufferLoggerHook()
		logger.Hooks.Add(p.logger)

		p.err = p.CreateNamespaces(logger)
		p.mutex.Unlock()
		p.state = StateReady
	}()

	return nil
}

func (p *Process) Status() (int, *BufferLogger, error) {
	return p.state, p.logger, p.err
}

// Create namespaces
func (p *Process) CreateNamespaces(logger *log.Logger) error {
	labelSelector, err := labels.Parse("kubehub/enable=true,kubehub/project=" + p.Config.Project)
	if err != nil {
		logger.Errorf("Cannot create label %v", err)
		return err
	}

	kubeNs, err := p.Kube.Namespaces().List(labelSelector, fields.Everything())
	if err != nil {
		logger.Errorf("Cannot list namespaces %v", err)
		return err
	}

	kubeNsIndex := IndexMapList(
		func(ns interface{}) string {
			return ns.(api.Namespace).Name
		},
		func(ns interface{}) interface{} {
			return &Entity{ns, false}
		},
		kubeNs.Items,
	)

	for _, ns := range p.Config.Namespaces {
		name := p.Config.Project + "-" + ns.Name
		nsLogger := logger.WithFields(log.Fields{"namespace": ns.Name})

		nsLogger.Info("Processing namespace")

		setNs := func(ns api.Namespace) api.Namespace {
			ns.ObjectMeta.Name = name
			ns.ObjectMeta.Labels = map[string]string{
				"kubehub/enable":  "true",
				"kubehub/project": p.Config.Project,
			}
			return ns
		}

		namespace := setNs(api.Namespace{})
		if val, ok := kubeNsIndex[name]; ok {
			nsLogger.Info("Updating namespace")

			val.(*Entity).Processed = true
			currentNs := setNs(val.(*Entity).Value.(api.Namespace))
			_, err := p.Kube.Namespaces().Update(&currentNs)
			if err != nil {
				nsLogger.Errorf("Cannot update namespace %v", err)
				continue
			}

			if p.CreateApps(ns, logger) != nil {
				nsLogger.Errorf("Cannot create apps")
			}
		} else {
			nsLogger.Info("Creating namespace")

			_, err := p.Kube.Namespaces().Create(&namespace)
			if err != nil {
				nsLogger.Errorf("Cannot create namespace %v", err)
				continue
			}

			if p.CreateApps(ns, logger) != nil {
				nsLogger.Errorf("Cannot create apps")
			}
		}
	}

	notProcessed := Filter(func(el interface{}) bool {
		return !el.(*Entity).Processed
	}, Values(kubeNsIndex))
	for _, ns := range notProcessed {
		ns := ns.(*Entity).Value.(api.Namespace)
		logger.WithFields(log.Fields{"namespace": ns.Name}).Info("Deleting namespace")
		// For some reason have to call delete twice
		err := p.Kube.Namespaces().Delete(ns.Name)
		err = p.Kube.Namespaces().Delete(ns.Name)
		if err != nil {
			logger.WithFields(log.Fields{"namespace": ns.Name}).Errorf("Cannot delete namespace %v", err)
		}
	}

	return nil
}

// Creates apps for namespace
func (p *Process) CreateApps(ns Namespace, logger *log.Logger) error {
	nsName := p.Config.Project + "-" + ns.Name
	nsLogger := logger.WithFields(log.Fields{"namespace": ns.Name})

	appGroups := IndexList(func(group interface{}) string {
		return group.(ApplicationGroup).Name
	}, p.Config.ApplicationGroups)

	apps := IndexList(func(app interface{}) string {
		return app.(Application).Name
	}, p.Config.Applications)

	templates := IndexList(func(tpl interface{}) string {
		return tpl.(Template).Name
	}, p.Config.Templates)

	labelSelector, err := labels.Parse("kubehub/enable=true,kubehub/project=" + p.Config.Project)
	if err != nil {
		nsLogger.Errorf("Cannot create label %v", err)
		return err
	}

	kubeSc, err := p.Kube.Services(nsName).List(labelSelector)
	if err != nil {
		nsLogger.Errorf("Cannot list services %v", err)
		return err
	}

	// Index by service label kubehub/name
	kubeScIndex := IndexMapList(
		func(ns interface{}) string {
			return ns.(api.Service).Name
		},
		func(ns interface{}) interface{} {
			return &Entity{ns, false}
		},
		kubeSc.Items,
	)

	kubeRc, err := p.Kube.ReplicationControllers(nsName).List(labelSelector)
	if err != nil {
		nsLogger.Errorf("Cannot list replication controllers %v", err)
		return err
	}

	// Index by replication controller label kubehub/name
	kubeRcIndex := IndexMapList(
		func(ns interface{}) string {
			krc := ns.(api.ReplicationController)
			if name, ok := krc.ObjectMeta.Labels["kubehub/name"]; ok {
				return name
			} else {
				return krc.Name
			}
		},
		func(ns interface{}) interface{} {
			return &Entity{ns, false}
		},
		kubeRc.Items,
	)

	createApp := func(group ApplicationGroup, app Application) error {
		appLogger := nsLogger.WithFields(log.Fields{"app": app.Name})

		// Merge tags from namespace, group and app
		tags := make(map[string]string)
		mergo.Merge(&tags, ns.Tags)
		mergo.MergeWithOverwrite(&tags, group.Tags)
		mergo.MergeWithOverwrite(&tags, app.Tags)

		setMeta := func(meta *api.ObjectMeta) {
			meta.Labels = map[string]string{
				"kubehub/enable":  "true",
				"kubehub/project": p.Config.Project,
				"kubehub/name":    app.Name,
			}
		}

		// Process service
		if app.Service != "" {
			scLogger := appLogger.WithFields(log.Fields{"template": app.Service})

			scLogger.Info("Processing service")

			template, ok := templates[app.Service].(Template)
			if !ok {
				err := errors.New("Template for service not found")
				scLogger.Error(err)
				return err
			}

			sc, _, err := template.Generate(p.Kube, tags)
			if err != nil {
				scLogger.Errorf("Cannot generate service template %v", err)
				return err
			}
			tplSc := sc.(*api.Service)
			setMeta(&tplSc.ObjectMeta)
			tplSc.Name = app.Name

			if entity, ok := kubeScIndex[app.Name].(*Entity); ok {
				sc := entity.Value.(api.Service)
				scLogger.Info("Updating service")

				if tplSc.Spec.PortalIP == "" {
					tplSc.Spec.PortalIP = sc.Spec.PortalIP
					tplSc.ResourceVersion = sc.ResourceVersion
				}

				_, err := p.Kube.Services(sc.Namespace).Update(tplSc)
				if err != nil {
					scLogger.Errorf("Cannot update service %v", err)
					return err
				}

				entity.Processed = true
			} else {
				scLogger.Info("Creating service")
				_, err := p.Kube.Services(nsName).Create(tplSc)
				if err != nil {
					scLogger.Errorf("Cannot create service %v", err)
					return err
				}
			}
		}

		// Process replication controller
		if app.ReplicationController != "" {
			rcLogger := appLogger.WithFields(log.Fields{"template": app.ReplicationController})

			rcLogger.Info("Processing ReplicationController")

			template, ok := templates[app.ReplicationController].(Template)
			if !ok {
				err := errors.New("Template for rc not found")
				rcLogger.Error(err)
				return err
			}

			rc, _, err := template.Generate(p.Kube, tags)
			if err != nil {
				rcLogger.Error("Cannot generate rc template %v", err)
				return err
			}
			tplRc := rc.(*api.ReplicationController)
			setMeta(&tplRc.ObjectMeta)
			rcLogger = rcLogger.WithFields(log.Fields{"rc": tplRc.Name})

			if entity, ok := kubeRcIndex[app.Name].(*Entity); ok {
				rc := entity.Value.(api.ReplicationController)

				rcLogger.Info("Updating rc")

				if tplRc.Name != rc.Name {
					rcLogger.WithFields(log.Fields{"to": tplRc.Name}).Info("Rollupdating")

					buf := bytes.NewBuffer(nil)
					updater := kubectl.NewRollingUpdater(nsName, p.Kube)
					err := updater.Update(buf, &rc, tplRc, 1*time.Second, 1*time.Second, 10*time.Second)
					if err != nil {
						rcLogger.Error("Problem with rolling update %v", err)
						return err
					}
				} else {
					rc.Spec.Replicas = tplRc.Spec.Replicas
					_, err := p.Kube.ReplicationControllers(rc.Namespace).Update(&rc)
					if err != nil {
						rcLogger.Error("Cannot update replication controller  %v", err)
						return err
					}
				}

				entity.Processed = true
			} else {
				rcLogger.Info("Creating replication controller")
				_, err := p.Kube.ReplicationControllers(nsName).Create(tplRc)
				if err != nil {
					rcLogger.Errorf("Cannot create replication controller %v", err)
					return err
				}
			}
		}

		return nil
	}

	var appErr error
	if group, ok := appGroups[ns.ApplicationGroup].(ApplicationGroup); ok {
		wait := sync.WaitGroup{}
		wait.Add(len(group.Applications))

		for _, app := range group.Applications {
			if app, ok := apps[app].(Application); ok {
				go func() {
					err := createApp(group, app)
					if err != nil {
						appErr = err
					}
					wait.Done()
				}()
			} else {
				err := errors.New("App not found " + app.Name)
				nsLogger.Error(err)
				return err
			}
		}

		wait.Wait()
	} else {
		nsLogger.WithFields(log.Fields{"group": group.Name}).Error("Application group not found")
	}

	// Garbage collect services
	gcServices := Filter(func(el interface{}) bool {
		return !el.(*Entity).Processed
	}, Values(kubeScIndex))
	for _, sc := range gcServices {
		sc := sc.(*Entity).Value.(api.Service)

		nsLogger.WithFields(log.Fields{"service": sc.Name}).Info("Deleting service")
		err := p.Kube.Services(sc.Namespace).Delete(sc.Name)
		if err != nil {
			nsLogger.WithFields(log.Fields{"service": sc.Name}).Errorf("Cannot delete service %v", err)
			appErr = err
		}
	}

	// Garbage collect replication controllers
	gcRc := Filter(func(el interface{}) bool {
		return !el.(*Entity).Processed
	}, Values(kubeRcIndex))
	for _, rc := range gcRc {
		rc := rc.(*Entity).Value.(api.ReplicationController)
		nsLogger.WithFields(log.Fields{"rc": rc.Name}).Info("Deleting rc")

		rc.Spec.Replicas = 0
		p.Kube.ReplicationControllers(rc.Namespace).Update(&rc)
		if err != nil {
			nsLogger.WithFields(log.Fields{"rc": rc.Name}).Errorf(
				"Cannot delete rc, cannot set replicas to 0 %v", err)
			appErr = err
		}

		err := p.Kube.ReplicationControllers(rc.Namespace).Delete(rc.Name)
		if err != nil {
			nsLogger.WithFields(log.Fields{"rc": rc.Name}).Errorf("Cannot delete rc %v", err)
			appErr = err
		}
	}

	return appErr
}
