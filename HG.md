


./hack/local-down-karmada.sh

./hack/local-up-karmada-hg.sh



```shell

export VERSION="latest"
export REGISTRY="docker.io/karmada"
make images GOOS="linux" 
kind load docker-image --name=karmada-host docker.io/karmada/karmada-controller-manager:latest 

```