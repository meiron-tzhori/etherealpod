# EtherealPod Operator

A Kubernetes Operator built using Kubebuilder that introduces a custom resource named **EtherealPod** and enforces strict reconciliation logic.

The operator guarantees that **exactly one managed Pod exists per EtherealPod resource**, and continuously reconciles cluster state to match the desired specification.

---

# Overview

The EtherealPod Operator extends Kubernetes using:

* A **Custom Resource Definition (CRD)**
* A **Controller** implementing reconciliation logic
* Ownership and garbage collection integration

The operator ensures:

* Exactly one managed Pod exists per `EtherealPod`
* If the Pod is deleted → it is recreated
* If multiple Pods exist → extras are removed
* If the Pod crashes → reconciliation restores desired state
* If the EtherealPod resource is deleted → owned Pods are cleaned automatically

---

# Architecture

The operator follows the standard Kubernetes controller pattern:

1. User creates an `EtherealPod` custom resource.
2. The controller watches for changes.
3. The reconciliation loop:

   * Lists owned Pods
   * Compares observed vs desired state
   * Creates / deletes Pods as necessary
4. Status is updated to reflect current observed state.

Ownership is configured using:

```
controllerutil.SetControllerReference
```

This ensures proper cascading deletion via Kubernetes garbage collection.

---

# Project Structure

```
etherealpod/
├── api/                 # CRD definitions (v1alpha1)
├── internal/controller/ # Reconciliation logic
├── config/              # CRD, RBAC, manager manifests
├── ep-test/             # Test manifests
├── Dockerfile
├── Makefile
└── main.go
```

---

# Installation & Deployment

## Prerequisites

* Kubernetes cluster (v1.24+ recommended)
* kubectl
* Docker
* Make
* Go (if building locally)

---

# Build & Deployment Workflow

The following steps describe the full build and deployment process in a standard Kubernetes environment.

---

## 1. Build Local Controller Binary (Optional)

```bash
make build
```

This compiles the controller locally using `go build`.
It is useful for local development or debugging.

---

## 2. Build Controller Docker Image

Choose an image name accessible to your Kubernetes cluster:

```bash
export IMG=your-registry/etherealpod-controller:latest
make docker-build IMG=$IMG
```

This builds the controller container image using the provided Dockerfile.

---

## 3. Push Image to Registry (if required)

If your cluster pulls images from a registry:

```bash
make docker-push IMG=$IMG
```

---

## 4. Install CRDs

```bash
make install
```

This:

* Generates CRD manifests using controller-gen
* Applies the CRDs to the cluster

Verify:

```bash
kubectl get crd | grep etherealpods
```

---

## 5. Deploy the Controller to the Cluster

```bash
make deploy IMG=$IMG
```

This command:

* Applies RBAC configuration
* Deploys the controller-manager Deployment
* Applies kustomize configuration under `config/default`
* Configures the controller to use the provided image

Verify:

```bash
kubectl get pods -A | grep etherealpod
kubectl get deployments -A | grep controller
```

You should see the controller manager running.

---

# Usage

## Testing Creation of EtherealPod Resource
Assuming the namespace name for all commands ahead is 'ep-test'

Example:

```bash
kubctl create namespace ep-test
kubectl apply -n ep-test -f ep-test/ep-demo.yaml
```

Verify:

```bash
kubectl get etherealpods -n ep-test
kubectl get pods -n ep-test
```

Expected:

* Exactly one Pod is created and managed.

---

# Testing Reconciliation Scenarios

## 1. Manual Pod Deletion (Self-Healing)

Delete the managed Pod:

```bash
kubectl delete pod -n ep-test <pod-name>
```

Expected:

* The operator recreates a new Pod automatically.

---

## 2. Multiple Pods Enforcement

Apply an extra Pod:

```bash
kubectl apply -n ep-test -f ep-test/demo-extra-pod.yaml
```

Expected:

* If more than one matching Pod exists,
  reconciliation removes extras and restores exactly one.

---

## 3. Crash Scenario

Apply crash manifest:

```bash
kubectl apply -n ep-test -f ep-test/ep-crash.yaml
```

Observe:

```bash
kubectl get pods -n ep-test
kubectl describe pod -n ep-test <pod-name>
```

Expected:

* The Pod enters a failing state.
* The reconciliation loop restores desired state.

(The exact crash behavior depends on the test manifest.)

---

# Deleting the Resource

When the EtherealPod custom resource is deleted:

```bash
kubectl delete etherealpod <name> -n ep-test
```

Expected:

* The owned Pod is automatically removed due to owner reference.

---

# Status Handling

The operator updates the resource status to reflect the observed cluster state.

Check status:

```bash
kubectl get etherealpod <name> -n ep-test -o yaml
```

This allows observability into reconciliation results.

---

# Makefile Command Summary

For clarity, the main Make targets behave as follows:

| Command                     | Purpose                                            |
| --------------------------- | -------------------------------------------------- |
| `make build`                | Builds the controller binary locally               |
| `make docker-build IMG=...` | Builds the controller container image              |
| `make docker-push IMG=...`  | Pushes the image to registry                       |
| `make install`              | Installs CRDs into the cluster                     |
| `make deploy IMG=...`       | Deploys controller manager to cluster              |
| `make run`                  | Runs controller locally against current kubeconfig |

---

# Design Decisions

* Kubebuilder was used to follow Kubernetes controller-runtime best practices.
* Ownership references ensure proper lifecycle handling.
* The controller enforces strict "exactly one" semantics.
* Logic is namespace-scoped.
* No admission webhooks were implemented.

---

# Limitations

* Designed for single managed Pod per resource.
* Does not implement scaling logic.
* Assumes standard Kubernetes garbage collection behavior.
* No HA setup for the controller.

---

