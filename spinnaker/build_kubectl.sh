#!/bin/bash

USER=spinnaker
CLUSTER=cluster-1

echo "Retrieving token for the ${USER} user" >&2
TOKEN=`kubectl get secret $(kubectl get sa ${USER} -o jsonpath='{.secrets[0].name}') -o jsonpath='{.data.token}' | base64 --decode`
echo "Retrieving certificate authority for the ${CLUSTER} cluster" >&2
SERVER=`kubectl config view --flatten --minify -o jsonpath="{.clusters[0].cluster.server}"`
CA=`kubectl config view --minify --flatten | grep certificate-authority-data | awk '{print $2}'`

echo "apiVersion: v1
kind: Config
users:
- name: ${USER}
  user:
    token: ${TOKEN}
clusters:
- cluster:
    certificate-authority-data: ${CA}
    server: ${SERVER}
  name: ${CLUSTER}
contexts:
- context:
    cluster: ${CLUSTER}
    user: ${USER}
  name: ${CLUSTER}
current-context: ${CLUSTER}
"
