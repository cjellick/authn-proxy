## Prereqs

Running k8s cluster with RBAC turned on and the PodSecurityPolicy admission controller enabled
```
minikube -p demo start --vm-driver kvm --extra-config=apiserver.Authorization.Mode=RBAC
```

Assert RBAC is on and an unauthorized user cannot access the cluster:
```
kubectl  --as cjellick get nodes
```

### CRDs
If Rancher isn't going to create the authz CRDs for you, you need to create them:
```
kubectl create -f crds.yaml 

### Contents of crds.yaml
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: projectroletemplates.authorization.cattle.io
spec:
  group: authorization.cattle.io
  version: v1
  scope: Cluster
  names:
    plural: projectroletemplates
    singular: projectroletemplate
    kind: ProjectRoleTemplate
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: podsecuritypolicytemplates.authorization.cattle.io
spec:
  group: authorization.cattle.io
  version: v1
  scope: Cluster
  names:
    plural: podsecuritypolicytemplates
    singular: podsecuritypolicytemplate
    kind: PodSecurityPolicyTemplate
---

apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: projectroletemplatebindings.authorization.cattle.io
spec:
  group: authorization.cattle.io
  version: v1
  scope: Cluster
  names:
    plural: projectroletemplatebindings
    singular: projectroletemplatebinding
    kind: ProjectRoleTemplateBinding
```

### cluster-agent
If Rancher isn't deploying the cluster-agent, you need to deploy it manually.

First, it needs a servicea account with cluster-admin level privileges
```
kubectl create sa svc-cluster-admin
kubectl create clusterrolebinding svc-cluster-admin-binding --clusterrole=cluster-admin --serviceaccount=default:svc-cluster-admin
```

Now, you can deploy the cluster-agent

Note that when creating CRDs, I didn't create the ones for nodesyncer or healthsyncer, so if Rancher isn't creating the CRDs yourself, you either need to create those, or comment out those controllers thusly:
```
diff --git a/controller/controllers.go b/controller/controllers.go
index db7b33e..d0dbff1 100644
--- a/controller/controllers.go
+++ b/controller/controllers.go
@@ -2,13 +2,11 @@ package controller
 
 import (
 	"github.com/rancher/cluster-agent/controller/authz"
-	"github.com/rancher/cluster-agent/controller/healthsyncer"
-	"github.com/rancher/cluster-agent/controller/nodesyncer"
 	"github.com/rancher/types/config"
 )
 
 func Register(workload *config.WorkloadContext) {
-	nodesyncer.Register(workload)
-	healthsyncer.Register(workload)
+	// nodesyncer.Register(workload)
+	// healthsyncer.Register(workload)
 	authz.Register(workload)
 }
```
The image `cjellick/cluster-agent:dev` that I use in the following deployment has this change baked in.

```
kubectl create -f cluster-agent.yaml

### Contents of cluster-agent.yaml
apiVersion: apps/v1beta2
kind: Deployment
metadata:
  name: cluster-agent
  labels:
    svc: cluster-agent
spec:
  replicas: 1
  selector:
    matchLabels:
      svc: cluster-agent
  template:
    metadata:
      labels:
        svc: cluster-agent
    spec:
      serviceAccountName: svc-cluster-admin
      containers:
        - name: cluster-agent
          image: cjellick/cluster-agent:dev
          imagePullPolicy: Always

```

### authn-proxy
If the authn-proxy isn't baked into rancher, you need to deploy it manually.

First, you need to create a secret for the selfsigned certs that the proxy will use for https:
```
### Create the certs:
openssl req -x509 -nodes -days 365 -newkey rsa:2048 -subj '/O=Rancher Labs/L=Tempe/ST=AZ/C=US' -keyout selfsigned.key -out selfsigned.crt

### Create the secret:
kubectl create secret generic authn-certs --from-file=selfsigned.crt --from-file=selfsigned.key

```

Second you need to create a config map for the server properties:
```
kubectl create configmap server-properties --from-file server.properties

### Contents of server.properties:
log.level=debug
frontend.http.host=0.0.0.0:9999
frontend.https.host=0.0.0.0:9443
frontend.ssl.cert.path=/var/run/cattle.io/certs/selfsigned.crt
frontend.ssl.key.path=/var/run/cattle.io/certs/selfsigned.key
```

Now, you can create the deployment (and NodePort service for exposing a port):
```
kubectl create -f authn.yaml

### Contens of autn.yaml
apiVersion: apps/v1beta2
kind: Deployment
metadata:
  name: authn-proxy
  labels:
    svc: autn-proxy
spec:
  replicas: 1
  selector:
    matchLabels:
      svc: authn-proxy
  template:
    metadata:
      labels:
        svc: authn-proxy
    spec:
      serviceAccountName: svc-cluster-admin
      containers:
        - name: authn-proxy
          image: cjellick/authn-proxy:dev
          imagePullPolicy: Always
          volumeMounts:
          - name: certs
            mountPath: /var/run/cattle.io/certs
            readOnly: true
          - name: server-props
            mountPath: /var/run/cattle.io/config
            readOnly: true
      volumes:
      - name: certs
        secret:
          secretName: authn-certs
      - name: server-props
        configMap:
          name: server-properties
---
apiVersion: v1
kind: Service
metadata:
  name: authn-svc
  labels:
    svc: authn-proxy
spec:
  type: NodePort
  selector:
    svc: authn-proxy
  ports:
  - port: 9443
    protocol: TCP
    name: https
  - port: 9999
    protocol: TCP
    name: http
```

Now, configure your kubeconfig file to point to the autn-proxy
```
### Copy real config to a new one you'll use
cp ~/.kube/config kube-config

###Tweak it to not have a cert-authority, to skip verify certs, and to point at the nodeport:
diff --git a/kube-config b/kube-config
index 81ffdd1..330c4f4 100644
--- a/kube-config
+++ b/kube-config
@@ -1,8 +1,8 @@
 apiVersion: v1
 clusters:
 - cluster:
-    certificate-authority: /home/cjellick/.minikube/ca.crt
-    server: https://192.168.43.80:8443
+    server: https://192.168.43.80:32487
+    insecure-skip-tls-verify: true
   name: demo
```

You could also manually tweak the kube-config to use username and password, or you can just pass them on the command line:
```
kubectl --kubeconfig kube-config --username cjellick --password group1,group2 get pods
```
If you want to do it in the config file, it'll look like this:
```
contexts:
- context:
    cluster: demo
    user: cjellick
  name: cjellick
current-context: cjellick
kind: Config
preferences: {}
users:
- name: cjellick
  user:
    as-user-extra: {}
    password: group1:group2
    username: cjellick
```

Finally, there's a bug () in 1.8 where not all swagger api paths are allowed. This results in a warning in kubectl. You can work around with this:
```
(authn-proxy)crs$ kubectl create clusterrole swagger --verb=get --non-resource-url=/swagger-2.0.0.pb-v1 --non-resource-url=/swagger.json
(authn-proxy)crs$ kubectl create clusterrolebinding swagger --clusterrole=swagger --group=system:authenticated --group=system:unauthenticated
```
