


./hack/local-down-karmada.sh

./hack/local-up-karmada-hg.sh



```shell

export VERSION="latest"
export REGISTRY="docker.io/karmada"
make images GOOS="linux" 
kind load docker-image --name=karmada-host docker.io/karmada/karmada-controller-manager:latest 

```

                       
docker pull --platform=amd64 registry.k8s.io/kube-apiserver:v1.25.4
docker tag  registry.k8s.io/kube-apiserver:v1.25.4  hub.jdcloud.com/jdos-edge/kube-apiserver:v1.25.4-amd64
docker push  hub.jdcloud.com/jdos-edge/kube-apiserver:v1.25.4-amd64

docker pull --platform=amd64 registry.k8s.io/kube-controller-manager:v1.25.4
docker tag  registry.k8s.io/kube-controller-manager:v1.25.4  hub.jdcloud.com/jdos-edge/kube-controller-manager:v1.25.4-amd64
docker push  hub.jdcloud.com/jdos-edge/kube-controller-manager:v1.25.4-amd64


docker pull --platform=amd64 registry.k8s.io/etcd:3.5.9-0
docker tag registry.k8s.io/etcd:3.5.9-0  hub.jdcloud.com/jdos-edge/etcd:3.5.9-0-amd64
docker push hub.jdcloud.com/jdos-edge/etcd:3.5.9-0-amd64