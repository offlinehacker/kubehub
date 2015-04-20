package main

import (
	log "github.com/Sirupsen/logrus"
	"github.com/ant0ine/go-json-rest/rest"
	"net/http"
	"reflect"
	"sync"
)

type Api struct {
	Process    *Process
	Api        *rest.Api
	lock       sync.RWMutex
	commitLock sync.Mutex
}

func NewApi(Process *Process) (*Api, error) {
	api := Api{}
	api.Process = Process

	// Crate api
	api.Api = rest.NewApi()
	router, err := rest.MakeRouter(
		rest.Get("/apps", api.getResources(&Process.Config.Applications)),
		rest.Get("/apps/:name", api.getResource(&Process.Config.Applications)),
		rest.Post("/apps", api.createResource(&Process.Config.Applications)),
		rest.Put("/apps/:name", api.updateResource(&Process.Config.Applications)),
		rest.Delete("/apps/:name", api.deleteResource(&Process.Config.Applications)),
		rest.Get("/groups", api.getResources(&Process.Config.ApplicationGroups)),
		rest.Get("/groups/:name", api.getResource(&Process.Config.ApplicationGroups)),
		rest.Post("/groups", api.createResource(&Process.Config.ApplicationGroups)),
		rest.Put("/groups/:name", api.updateResource(&Process.Config.ApplicationGroups)),
		rest.Delete("/groups/:name", api.deleteResource(&Process.Config.ApplicationGroups)),
		rest.Get("/namespaces", api.getResources(&Process.Config.Namespaces)),
		rest.Get("/namespaces/:name", api.getResource(&Process.Config.ApplicationGroups)),
		rest.Post("/namespaces", api.createResource(&Process.Config.Namespaces)),
		rest.Put("/namespaces/:name", api.updateResource(&Process.Config.Namespaces)),
		rest.Delete("/namespaces/:name", api.deleteResource(&Process.Config.Namespaces)),
		rest.Get("/templates", api.getResources(&Process.Config.Templates)),
		rest.Get("/templates/:name", api.getResource(&Process.Config.ApplicationGroups)),
		rest.Post("/templates", api.createResource(&Process.Config.Templates)),
		rest.Put("/templates/:name", api.updateResource(&Process.Config.Templates)),
		rest.Delete("/templates/:name", api.deleteResource(&Process.Config.Templates)),
		rest.Post("/hook/newtag", api.newtag),
		rest.Post("/deploy", api.commit),
		rest.Get("/deploy", api.status),
	)
	if err != nil {
		log.Errorf("Cannot create router %v", err)
		return nil, err
	}

	api.Api.SetApp(router)
	api.lock = sync.RWMutex{}
	return &api, nil
}

func (a *Api) getResources(resource interface{}) rest.HandlerFunc {
	return func(w rest.ResponseWriter, r *rest.Request) {
		a.lock.RLock()
		w.WriteJson(resource)
		a.lock.RUnlock()
	}
}

func (a *Api) getResource(resources interface{}) rest.HandlerFunc {
	return func(w rest.ResponseWriter, r *rest.Request) {
		a.lock.RLock()
		resourceIndex := IndexList(
			func(resource interface{}) string {
				immutable := reflect.ValueOf(resource)
				return immutable.FieldByName("Name").String()
			},
			reflect.ValueOf(resources).Elem().Interface(),
		)
		a.lock.RUnlock()

		name := r.PathParam("name")
		if resource, ok := resourceIndex[name]; ok {
			w.WriteJson(resource)
		} else {
			rest.Error(w, "Resource not found", http.StatusNotFound)
		}
	}
}

func (a *Api) deleteResource(resources interface{}) rest.HandlerFunc {
	return func(w rest.ResponseWriter, r *rest.Request) {
		a.lock.Lock()

		reflected := reflect.ValueOf(resources).Elem()
		out := reflect.MakeSlice(reflected.Type(), 0, reflected.Len())
		found := false

		name := r.PathParam("name")
		for i := 0; i < reflected.Len(); i++ {
			if reflected.Index(i).FieldByName("Name").String() != name {
				out.Set(reflect.Append(out, reflected.Index(i)))
			} else {
				found = true
			}
		}
		reflected.Set(out)
		a.lock.Unlock()

		if !found {
			rest.Error(w, "Resource not found", http.StatusNotFound)
		}
	}
}

func (a *Api) createResource(resources interface{}) rest.HandlerFunc {
	return func(w rest.ResponseWriter, r *rest.Request) {
		a.lock.Lock()
		reflected := reflect.ValueOf(resources).Elem()

		resourceIndex := IndexList(
			func(resource interface{}) string {
				immutable := reflect.ValueOf(resource)
				return immutable.FieldByName("Name").String()
			},
			reflected.Interface(),
		)

		newValue := reflect.New(reflected.Type().Elem())
		err := r.DecodeJsonPayload(newValue.Interface())
		if err != nil {
			rest.Error(w, "Cannot decode resource:"+err.Error(), http.StatusBadRequest)
			return
		}

		if _, ok := resourceIndex[newValue.Elem().FieldByName("Name").String()]; ok {
			rest.Error(w, "Resource already exists", http.StatusConflict)
			return
		}

		reflected.Set(reflect.Append(reflected, newValue.Elem()))
		w.WriteJson(newValue.Interface())
		a.lock.Unlock()
	}
}

func (a *Api) updateResource(resources interface{}) rest.HandlerFunc {
	return func(w rest.ResponseWriter, r *rest.Request) {
		a.lock.Lock()

		reflected := reflect.ValueOf(resources).Elem()

		name := r.PathParam("name")
		for i := 0; i < reflected.Len(); i++ {
			if reflected.Index(i).FieldByName("Name").String() == name {
				newValue := reflect.New(reflected.Type().Elem())

				err := r.DecodeJsonPayload(newValue.Interface())
				if err != nil {
					rest.Error(w, "Cannot decode resource:"+err.Error(), http.StatusBadRequest)
					a.lock.Unlock()
					return
				}

				reflected.Index(i).Set(newValue.Elem())
				w.WriteJson(newValue.Interface())
				a.lock.Unlock()
				return
			}
		}
		a.lock.Unlock()

		rest.Error(w, "Resource not found", http.StatusNotFound)
	}
}

// Applies new configuration
func (a *Api) commit(w rest.ResponseWriter, r *rest.Request) {
	if err := a.Process.Commit(); err != nil {
		rest.Error(w, "Cannot commit changes:"+err.Error(), http.StatusBadRequest)
	}
}

func (a *Api) status(w rest.ResponseWriter, r *rest.Request) {
	state, logger, err := a.Process.Status()
	errors := []map[string]interface{}{}
	logs := []map[string]interface{}{}
	if logger != nil {
		for _, entry := range logger.Entries {
			if entry.Level == log.ErrorLevel {
				errors = append(errors, map[string]interface{}{"msg": entry.Message, "fields": entry.Data})
			} else {
				logs = append(logs, map[string]interface{}{"msg": entry.Message, "fields": entry.Data})
			}
		}
	}
	w.WriteJson(map[string]interface{}{"state": state, "err": err, "errors": errors, "logs": logs})
}

func (a *Api) newtag(w rest.ResponseWriter, r *rest.Request) {
	name := r.URL.Query().Get("image")
	tag := r.URL.Query().Get("tag")
	imageFound := false

	log.Debugf("Newtag %v %v", name, tag)

	for idx, app := range a.Process.Config.Applications {
		image, ok := app.Tags["image"]
		if !ok || image != name {
			continue
		}

		_, ok = app.Tags["tag"]
		if !ok {
			continue
		}

		autoupdate, ok := app.Tags["autoupdate"]
		if !ok || autoupdate != "true" {
			continue
		}

		a.Process.Config.Applications[idx].Tags["tag"] = tag

		if err := a.Process.Commit(); err != nil {
			rest.Error(w, "Cannot commit changes:"+err.Error(), http.StatusInternalServerError)
			continue
		}

		imageFound = true
	}

	if !imageFound {
		rest.Error(w, "Image not found", http.StatusNotFound)
	}
}

func (a *Api) Serve(host string) error {
	log.Info("Listening on", host)
	err := http.ListenAndServe(host, a.Api.MakeHandler())
	if err != nil {
		log.Errorf("Cannot listen %v", err)
	}

	return err
}
