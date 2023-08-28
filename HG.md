





```shell

cat<<EOF > kamada-val-tmp.yaml

# 安装哪些组件
components: ["descheduler"]

controllerManager:
  featureGates:
    PropagateDeps: false
    CustomizedClusterResourceModeling: false

EOF

```