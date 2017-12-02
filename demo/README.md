### Demoing auth functionality

### Pre-requisites
This assumes you've done the setup in [setup.md](setup.md). Some or all of that setup may be handled by Rancher, depending on how along the intregration effort is.
At the very least, you'll need to have a kubeconfig that points at the authn-proxy instead of directly at kubernetes. Here's a diff showing what that change would look like:
```
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

This demo assumes you have two kubeconfig files:
- The default at ~/.kube/config, which is configured to point directly at the k8s end point and to use the default user that has cluster-admin level privileges. This will be used to add another cluster-admin user
- A local one at ./kube-config, which is configured to point at the authn-proxy. This will be used to demo the authn-proxy functionality.


### A little bootstrapping
Let's bootstrap in a clusterrole binding for an admin user:
```
kubectl create clusterrolebinding admin --clusterrole cluster-admin --user=dave-the-admin
```
Now, all further admin operations will done be using the dave-the-admin user.

The authn-proxy sitting in front of kubernetes can authenticate this user. This **demo** version of the authn-proxy just authenticates all users. The Basic Auth username passed in the request will be set as the k8s user and the password will be interpretted as a comma delimted list of groups. The real version would take the username and password and authenticate them against an external system like LDAP and get the user's groups from there.

You can see the authorization working. Make a request as dave-the-admin and one as tom-the-wannabe and see the difference:
```
kubectl --kubeconfig ./kube-config --username=dave-the-admin --password=pass get nodes
NAME      STATUS    ROLES     AGE       VERSION
demo      Ready     <none>    2h        v1.8.0

kubectl --kubeconfig ./kube-config --username=tom-the-wannabe --password=pass get nodes
Error from server (Forbidden): nodes is forbidden: User "tom-the-wannabe" cannot list nodes at the cluster scope
```

### Creating projects and namespaces
Now, Tom will create four namespaces: dev, test, staging, and prod. Dev and test should have very permissive access controls, so we'll group them into a project called `development`. Staging and prod should have very strict access controls, so we'll group them into a project called `production`.

Typically, Rancher would handle assigning namespaces to projects, but this demo is designed to work without the Rancher API server running, So we need to emulate that by adding the appropriate label.
```
kubectl --kubeconfig ./kube-config --username=dave-the-admin create ns dev
kubectl --kubeconfig ./kube-config --username=dave-the-admin label ns dev 'io.cattle.field.projectId=development'
kubectl --kubeconfig ./kube-config --username=dave-the-admin create ns test
kubectl --kubeconfig ./kube-config --username=dave-the-admin label ns test 'io.cattle.field.projectId=development'

kubectl --kubeconfig ./kube-config --username=dave-the-admin create ns staging
kubectl --kubeconfig ./kube-config --username=dave-the-admin label ns staging 'io.cattle.field.projectId=production'
kubectl --kubeconfig ./kube-config --username=dave-the-admin create ns prod
kubectl --kubeconfig ./kube-config --username=dave-the-admin label ns prod 'io.cattle.field.projectId=production'
```

The first thing Tom wants to do is define a ProjectRole that gives broad access. It will be used in the development project

First, let's prove a developer currently cannot access any of the namespaces in the deverloper project:
```
kubectl --kubeconfig ./kube-config --username=craig-the-dev --password=developers -n dev get pods
the server doesn't have a resource type "pods"
```

Now, create a ProjectRoletemplate representing the builtin role edit
```
kubectl --kubeconfig ./kube-config --username=dave-the-admin --password=pass create -f role-edit.yaml

### Contents of role-edit.yaml
apiVersion: authorization.cattle.io/v1
kind: ProjectRoleTemplate
metadata:
  name: edit
rules:
- apiGroups:
  - ""
  resources:
  - pods
  - pods/attach
  - pods/exec
  - pods/portforward
  - pods/proxy
  verbs:
  - create
  - delete
  - deletecollection
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - configmaps
  - endpoints
  - persistentvolumeclaims
  - replicationcontrollers
  - replicationcontrollers/scale
  - secrets
  - serviceaccounts
  - services
  - services/proxy
  verbs:
  - create
  - delete
  - deletecollection
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - bindings
  - events
  - limitranges
  - namespaces/status
  - pods/log
  - pods/status
  - replicationcontrollers/status
  - resourcequotas
  - resourcequotas/status
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - namespaces
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - serviceaccounts
  verbs:
  - impersonate
- apiGroups:
  - apps
  resources:
  - deployments
  - deployments/rollback
  - deployments/scale
  - statefulsets
  verbs:
  - create
  - delete
  - deletecollection
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - autoscaling
  resources:
  - horizontalpodautoscalers
  verbs:
  - create
  - delete
  - deletecollection
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - batch
  resources:
  - cronjobs
  - jobs
  verbs:
  - create
  - delete
  - deletecollection
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - extensions
  resources:
  - daemonsets
  - deployments
  - deployments/rollback
  - deployments/scale
  - ingresses
  - replicasets
  - replicasets/scale
  - replicationcontrollers/scale
  verbs:
  - create
  - delete
  - deletecollection
  - get
  - list
  - patch
  - update
  - watch

### You can see the ProjectRoleTemplate was created:
kubectl --kubeconfig ./kube-config --username=dave-the-admin --password=pass get projectroletemplates.authorization.cattle.io
```

Now, create the ProjectRoleTempalteBinding. This will give the group developers the edit role in all namespaces in the development project
```
kubectl --kubeconfig ./kube-config --username=dave-the-admin --password=pass create -f devs.yaml 

### Contents of devs.yaml
apiVersion: authorization.cattle.io/v1
kind: ProjectRoleTemplateBinding
metadata:
  name: developers-in-development
spec:
  subject:
    kind: Group
    name: developers
  projectName: development
  projectRoleTemplateName: edit

### You can see the custom resource was created:
kubectl --kubeconfig ./kube-config --username=dave-the-admin --password=pass get projectroletemplatebindings.authorization.cattle.io
```

See the role works!
```
### Members of the developer group can see pods in the dev and test namespaces (remember, we are faking groups in the password):
kubectl --kubeconfig ./kube-config --username=craig-the-dev --password=developers -n dev get pods
No resources found.

### Get pods in prod namespace (still denied)
kubectl --kubeconfig ./kube-config --username=craig-the-dev --password=developers -n prod get pods
Error from server (Forbidden): pods is forbidden: User "craig-the-dev" cannot list pods in the namespace "prod"

### Create a pod in the namespace
kubectl  --kubeconfig ./kube-config --username=craig-the-dev --password=developers -n dev create -f pod-priv.yaml

### Contents of pod-priv.yaml
apiVersion: v1
kind: Pod
metadata:
  name: ubuntu
  labels:
    name: ubuntu
spec:
  containers:
  - name: ubuntu
    image: ubuntu:14.04
    tty: true
    stdin: true
    securityContext:
      privileged: true

### The same command in prod fails:
kubectl  --kubeconfig ./kube-config --username=craig-the-dev --password=developers -n prod create -f pod-priv.yaml
Error from server (Forbidden): error when creating "pod-priv.yaml": pods is forbidden: User "craig-the-dev" cannot create pods in the namespace "prod"
```
