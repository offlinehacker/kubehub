# Kubehub

Kubernetes application hub

## Building

go build .

## Docker registry hook

```
docker-registry-server --user offlinehacker:test --on-tag "curl -X POST http://localhost:8081/hook/newtag?image=\${2/:*/}\&tag=\${2/*:/}"
```
