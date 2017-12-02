authn-proxy
========

An authentication proxy that usages kubernetes impersonation to forward requests to k8s using service account credentials and [impersonation headers](https://kubernetes.io/docs/admin/authentication/#user-impersonation).

## Building

`make`


## Running

For running locally:
```
TOKEN_PATH=<path to file> CONFIG_PATH=<path to file> authn-proxy
```

Both env vars can be omitted and default paths will be assumed:
- `TOKEN_PATH` - This should point to a file whose contents is a k8s service account token with cluster-admin level privileges. Defualt: `/var/run/secrets/kubernetes.io/serviceaccount/token`
- `CONFIG_PATH` - This should point to a properties file that containing additional params need to run the server. Default: `/var/run/cattle.io/config/server.properties`


Here's what should be in the `CONFIG_PATH` file:
```
log.level=debug

frontend.http.host=127.0.0.1:9999
frontend.https.host=127.0.0.1:9443
frontend.ssl.cert.path=selfsigned.crt
frontend.ssl.key.path=selfsigned.key

backend.scheme=https
backend.host=192.168.43.231:8443 
backend.ca.cert.path=/home/cjellick/.minikube/ca.crt
```
**NOTE**: `backend.scheme`, `backend.host`, & `backend.ca.cert` are **OPTIONAL** if you are running inside a k8s pod configured with an appropriate svc account. If omitted, the relevant information will be obtained via `rest.InClusterConfigi()` (which gets it from /var/run/secrets/kubernetes.io/serviceaccount).

For the frontend.ssl.* params, obviously, if you're running in a k8s pod and want to serve on https, you need to get the crt and key files into the pod. You can choose to not run the https server by dropping the frontend-https-\* parameters, but kubectl won't send authn headers if the endpoint is http.

### Using for (fake) authentication

The proxy will fake authenticate in two ways:
- If *Basic Auth* is sent, the username will be ther user and the password will be interpretted as a colon-delimited set of groups
- If a *Cookie* named `Authentication` is sent, its value will be interpretted as a base64 encoded string of the form `user:group1:groupn`


You can use basic auth with kubectl, but kubectl requires that SSL be turned on in order to pass the authentication header, so you have to provide the frontend-ssl parameters.
If you're using self-signed certs, you need to turn off cert verification. Here's a sample kubeconfig that works:
```
apiVersion: v1
clusters:
- cluster:
    server: https://127.0.0.1:9443
    insecure-skip-tls-verify: true
  name: master
contexts:
- context:
    cluster: master
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
If running in a k8s pod, the cluster.server value will need updated to the IP of the host and port the pod is running on.

## Deploying
For reference, here's a deployment that works (assuming the configmaps, secrets, and svc-accout are created:

**Note:** You'll have to update your kube-config to use the NodePort that k8s selected
```
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
      serviceAccountName: superuser
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
          secretName: selfsignedcerts1
      - name: server-props
        configMap:
          name: serverproperties1
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


Here's a(n interactive) one-liner for generating the self-signed cert files:
```
openssl req -x509 -nodes -days 365 -newkey rsa:2048 -keyout selfsigned.key -out selfsigned.crt
```

## License
Copyright (c) 2014-2017 [Rancher Labs, Inc.](http://rancher.com)

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

[http://www.apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0)

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
