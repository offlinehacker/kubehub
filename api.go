package main

import (
	log "github.com/Sirupsen/logrus"
	"github.com/emicklei/go-restful"
	"github.com/emicklei/go-restful/swagger"
	"net/http"
	"reflect"
	"sync"
)

type Api struct {
	Process    *Process
	lock       sync.RWMutex
	commitLock sync.Mutex
}

func NewApi(Process *Process) (*Api, error) {
	api := Api{}
	api.Process = Process

	api.lock = sync.RWMutex{}
	return &api, nil
}

func (a *Api) getResources(resource interface{}) restful.RouteFunction {
	return func(req *restful.Request, res *restful.Response) {
		a.lock.RLock()
		res.WriteEntity(resource)
		a.lock.RUnlock()
	}
}

func (a *Api) getResource(resources interface{}) restful.RouteFunction {
	return func(req *restful.Request, res *restful.Response) {
		a.lock.RLock()
		resourceIndex := IndexList(
			func(resource interface{}) string {
				immutable := reflect.ValueOf(resource)
				return immutable.FieldByName("Name").String()
			},
			reflect.ValueOf(resources).Elem().Interface(),
		)
		a.lock.RUnlock()

		name := req.PathParameter("name")
		if resource, ok := resourceIndex[name]; ok {
			res.WriteEntity(resource)
		} else {
			res.WriteErrorString(http.StatusNotFound, "Resource not found.")
		}
	}
}

func (a *Api) deleteResource(resources interface{}) restful.RouteFunction {
	return func(req *restful.Request, res *restful.Response) {
		a.lock.Lock()

		reflected := reflect.ValueOf(resources).Elem()
		out := reflect.MakeSlice(reflected.Type(), 0, reflected.Len())
		found := false

		name := req.PathParameter("name")
		for i := 0; i < reflected.Len(); i++ {
			if reflected.Index(i).FieldByName("Name").String() != name {
				out = reflect.Append(out, reflected.Index(i))
			} else {
				found = true
			}
		}
		reflected.Set(out)
		a.lock.Unlock()

		if !found {
			res.WriteErrorString(http.StatusNotFound, "Resource not found")
		}
	}
}

func (a *Api) createResource(resources interface{}) restful.RouteFunction {
	return func(req *restful.Request, res *restful.Response) {
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
		err := req.ReadEntity(newValue.Interface())
		if err != nil {
			res.WriteError(http.StatusBadGateway, err)
			return
		}

		if _, ok := resourceIndex[newValue.Elem().FieldByName("Name").String()]; ok {
			res.WriteErrorString(http.StatusConflict, "Resource already exists.")
			return
		}

		reflected.Set(reflect.Append(reflected, newValue.Elem()))
		res.WriteEntity(newValue.Interface())
		a.lock.Unlock()
	}
}

func (a *Api) updateResource(resources interface{}) restful.RouteFunction {
	return func(req *restful.Request, res *restful.Response) {
		a.lock.Lock()

		reflected := reflect.ValueOf(resources).Elem()

		name := req.PathParameter("name")
		for i := 0; i < reflected.Len(); i++ {
			if reflected.Index(i).FieldByName("Name").String() == name {
				newValue := reflect.New(reflected.Type().Elem())

				err := req.ReadEntity(newValue.Interface())
				if err != nil {
					res.WriteError(http.StatusBadRequest, err)
					a.lock.Unlock()
					return
				}

				reflected.Index(i).Set(newValue.Elem())
				res.WriteEntity(newValue.Interface())
				a.lock.Unlock()
				return
			}
		}
		a.lock.Unlock()

		res.WriteErrorString(http.StatusNotFound, "Resource not found.")
	}
}

// Applies new configuration
func (a *Api) commit(req *restful.Request, res *restful.Response) {
	if err := a.Process.Commit(); err != nil {
		res.WriteError(http.StatusInternalServerError, err)
	}
}

func (a *Api) status(req *restful.Request, res *restful.Response) {
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
	res.WriteEntity(map[string]interface{}{"state": state, "err": err, "errors": errors, "logs": logs})
}

func (a *Api) newtag(req *restful.Request, res *restful.Response) {
	name := req.QueryParameter("image")
	tag := req.QueryParameter("tag")
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
			res.WriteError(http.StatusInternalServerError, err)
			continue
		}

		imageFound = true
	}

	if !imageFound {
		res.WriteErrorString(http.StatusNotFound, "Image not found.")
	}
}

// Registers api and starts serving on specified host
func (api *Api) Serve(host string) error {
	log.Info("Listening on", host)

	// Apps
	ws := new(restful.WebService)
	ws.
		Path("/apps").
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON).
		Doc("Resource that defines which rc controller and service is app using")

	ws.Route(ws.GET("/").To(api.getResources(&api.Process.Config.Applications)).
		//docs
		Doc("get all apps").
		Operation("findAllApps").
		Returns(200, "OK", []Application{}))

	ws.Route(ws.POST("/").To(api.createResource(&api.Process.Config.Applications)).
		//docs
		Doc("creates an apps").
		Operation("createApp").
		Reads(Application{}))

	ws.Route(ws.GET("/{name}").To(api.getResource(&api.Process.Config.Applications)).
		//docs
		Doc("get an app").
		Operation("findApp").
		Param(ws.PathParameter("name", "name of on app").DataType("string")).
		Writes(Application{}))

	ws.Route(ws.PUT("/{name}").To(api.updateResource(&api.Process.Config.Applications)).
		//docs
		Doc("update an app").
		Operation("updateApp").
		Param(ws.PathParameter("name", "name of on app").DataType("string")).
		Reads(Application{}))

	ws.Route(ws.DELETE("/{name}").To(api.deleteResource(&api.Process.Config.Applications)).
		//docs
		Doc("delete an app").
		Operation("removeApp").
		Param(ws.PathParameter("name", "name of on app").DataType("string")))

	restful.Add(ws)

	// Groups
	ws = new(restful.WebService)
	ws.
		Path("/groups").
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON).
		Doc("Resource that groups a set of services")

	ws.Route(ws.GET("/").To(api.getResources(&api.Process.Config.ApplicationGroups)).
		//docs
		Doc("get all apps").
		Operation("findAllGroups").
		Returns(200, "OK", []ApplicationGroup{}))

	ws.Route(ws.POST("/").To(api.createResource(&api.Process.Config.ApplicationGroups)).
		//docs
		Doc("creates application group").
		Operation("createApplicationGroup").
		Reads(ApplicationGroup{}))

	ws.Route(ws.GET("/{name}").To(api.getResource(&api.Process.Config.ApplicationGroups)).
		//docs
		Doc("get an application group").
		Operation("findApp").
		Param(ws.PathParameter("name", "name of application group").DataType("string")).
		Writes(ApplicationGroup{}))

	ws.Route(ws.PUT("/{name}").To(api.updateResource(&api.Process.Config.ApplicationGroups)).
		//docs
		Doc("update appliction group").
		Operation("updateApp").
		Param(ws.PathParameter("name", "name of application group").DataType("string")).
		Reads(ApplicationGroup{}))

	ws.Route(ws.DELETE("/{name}").To(api.deleteResource(&api.Process.Config.ApplicationGroups)).
		//docs
		Doc("removes application group").
		Operation("removeApplicationGroup").
		Param(ws.PathParameter("name", "name of application group").DataType("string")))

	restful.Add(ws)

	// Namespaces
	ws = new(restful.WebService)
	ws.
		Path("/namespaces").
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON).
		Doc("Resource that defines namespaces (users)")

	ws.Route(ws.GET("/").To(api.getResources(&api.Process.Config.Namespaces)).
		//docs
		Doc("gets all namespaces").
		Operation("findNamespaces").
		Returns(200, "OK", []Namespace{}))

	ws.Route(ws.POST("/").To(api.createResource(&api.Process.Config.Namespaces)).
		//docs
		Doc("creates namespace").
		Operation("createNamespace").
		Reads(Namespace{}))

	ws.Route(ws.GET("/{name}").To(api.getResource(&api.Process.Config.Namespaces)).
		//docs
		Doc("gets a namespace").
		Operation("findNamespace").
		Param(ws.PathParameter("name", "name of the namespace").DataType("string")).
		Writes(Namespace{}))

	ws.Route(ws.PUT("/{name}").To(api.updateResource(&api.Process.Config.Namespaces)).
		//docs
		Doc("updates namespace").
		Operation("updateNamespace").
		Param(ws.PathParameter("name", "name of the namespace").DataType("string")).
		Reads(Namespace{}))

	ws.Route(ws.DELETE("/{name}").To(api.deleteResource(&api.Process.Config.Namespaces)).
		//docs
		Doc("removes namespace").
		Operation("removeNamespace").
		Param(ws.PathParameter("name", "name of the namespace").DataType("string")))

	restful.Add(ws)

	// Templates
	ws = new(restful.WebService)
	ws.
		Path("/templates").
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON).
		Doc("Resource for storage of templates")

	ws.Route(ws.GET("/").To(api.getResources(&api.Process.Config.Templates)).
		//docs
		Doc("gets all templates").
		Operation("findTemplates").
		Returns(200, "OK", []Template{}))

	ws.Route(ws.POST("/").To(api.createResource(&api.Process.Config.Templates)).
		//docs
		Doc("creates template").
		Operation("createTemplate").
		Reads(Template{}))

	ws.Route(ws.GET("/{name}").To(api.getResource(&api.Process.Config.Templates)).
		//docs
		Doc("gets a template").
		Operation("findTemplate").
		Param(ws.PathParameter("name", "name of the template").DataType("string")).
		Writes(Template{}))

	ws.Route(ws.PUT("/{name}").To(api.updateResource(&api.Process.Config.Templates)).
		//docs
		Doc("updates template").
		Operation("updateTemplate").
		Param(ws.PathParameter("name", "name of the template").DataType("string")).
		Reads(Template{}))

	ws.Route(ws.DELETE("/{name}").To(api.deleteResource(&api.Process.Config.Templates)).
		//docs
		Doc("removes template").
		Operation("removeTemplate").
		Param(ws.PathParameter("name", "name of the template").DataType("string")))

	restful.Add(ws)

	// Deployment
	ws = new(restful.WebService)
	ws.
		Path("/deploy").
		//Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON).
		Doc("Deployment of configuration")

	ws.Route(ws.POST("/").To(api.commit).
		//docs
		Doc("deploys config").Operation("deploy"))

	ws.Route(ws.GET("/").To(api.status).
		//docs
		Doc("gets deployment status").Operation("deploy"))

	restful.Add(ws)

	// Deployment hooks
	ws = new(restful.WebService)
	ws.
		Path("/hooks").
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON).
		Doc("Deployment hooks")

	ws.Route(ws.POST("/newtag").To(api.newtag).
		//docs
		Doc("updates all images that have autodeploy enabled").
		Operation("newtag"))

	restful.Add(ws)

	config := swagger.Config{
		WebServices:     restful.RegisteredWebServices(),
		ApiPath:         "/apidocs.json",
		SwaggerPath:     "/apidocs/",
		SwaggerFilePath: "swagger"}
	swagger.InstallSwaggerService(config)

	err := http.ListenAndServe(host, nil)
	if err != nil {
		log.Errorf("Cannot listen %v", err)
	}

	return err
}
