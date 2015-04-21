# Kubehub

Kubernetes application hub

## About

Simple paas, using kubernetes, for deployment of replicated, distributed apps

The goal of this project is to implement very simple alternative to openshift,
for deployment of apps for multiple user using same groups of applications.

## Model

```
Applications -> Application groups -> Namespaces
    /\
    ||
 Templates
```

## Building

```
make deps
make build
```

# Running

```
./kubehub --config=config.yaml
```

## Api

You can communicate with kubehub using a RESTful JSON API over HTTP. Kubehub
node usually listens on port 8081. All examples in this section assumes that
you are running kubehub api at localhost:8081

Api documentation can be found on [http://localhost:8081/apidocs/](http://localhost:8081/apidocs/)

## Docker registry integration

```
docker-registry-server --user offlinehacker:test --on-tag "curl -X POST http://localhost:8081/hook/newtag?image=\${2/:*/}\&tag=\${2/*:/}"
```

## License

MIT
