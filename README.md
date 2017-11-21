authn-proxy
========

An authentication proxy that usages kubernetes impersonation to forward requests to k8s using service account credentials and [impersonation headers](https://kubernetes.io/docs/admin/authentication/#user-impersonation).

## Building

`make`


## Running

For running locally:
```
authn-proxy --backend-addr 192.168.43.231:8443 --backend-scheme https --frontend-http-addr 127.0.0.1:9999 --ca-cert-path ~/.minikube/ca.crt --token-path token --frontend-https-addr 127.0.0.1:9443 --frontend-ssl-cert-path selfsigned.crt --frontend-ssl-key-path selfsigned.key
```

If you're running it inside of a k8s pod that has a service account configured, you can drop several of the paramters:
```
authn-proxy --frontend-http-addr :9999 --frontend-https-addr :9443 --frontend-ssl-cert-path selfsigned.crt --frontend-ssl-key-path selfsigned.key
```

Obviously, if you're running in a k8s pod and want to serve on https, you need to get the crt and key files into the pod. You can choose to not run the https server by dropping the frontend-https-\* parameters

Note that I haven't really tested running inside a pod yet.


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
