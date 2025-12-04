# Webhook Certificates in CAAPKW

## Overview

CAAPKW uses admission webhooks for validation and defaulting of `KubewardenAddon` and `KubewardenPolicy` resources. Webhooks require TLS certificates to secure communication between the Kubernetes API server and the webhook server.

## In-Cluster Deployment (Recommended)

When deploying the controller to a cluster using `make deploy`, webhook certificates are automatically managed by [cert-manager](https://cert-manager.io/):

1. **cert-manager** must be installed in the cluster
2. A `Certificate` resource is created in `config/certmanager/certificate.yaml`
3. cert-manager automatically generates a CA and webhook serving certificate
4. The CA bundle is injected into webhook configurations via the `cert-manager.io/inject-ca-from` annotation
5. Certificates are stored in a Secret and mounted into the controller pod

### Deployment Steps

```bash
# Install cert-manager (if not already installed)
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.2/cert-manager.yaml

# Wait for cert-manager to be ready
kubectl wait --for=condition=Available --timeout=2m -n cert-manager deployment/cert-manager
kubectl wait --for=condition=Available --timeout=2m -n cert-manager deployment/cert-manager-webhook
kubectl wait --for=condition=Available --timeout=2m -n cert-manager deployment/cert-manager-cainjector

# Build and deploy the controller
make docker-build IMG=myregistry/caapkw:dev
make deploy IMG=myregistry/caapkw:dev
```

For local Kind clusters, you need to load the image:

```bash
make docker-build IMG=caapkw-controller:dev
kind load docker-image caapkw-controller:dev --name <cluster-name>
cd config/manager && ../../bin/kustomize edit set image controller=caapkw-controller:dev
cd ../..
./bin/kustomize build config/default | kubectl apply -f -
```

## Local Development with `make run`

Running the controller locally with `make run` (or `go run ./cmd/main.go`) presents a challenge: the webhook server expects TLS certificates, but cert-manager only works in-cluster.

### Options for Local Development

#### Option 1: Deploy to Kind Cluster (Recommended)

Instead of running locally, deploy the controller to a local Kind cluster with cert-manager. This is the approach used in `scripts/demo.sh`:

1. Creates a Kind management cluster
2. Installs cert-manager
3. Builds and loads the controller image
4. Deploys the controller with `make deploy`

**Pros:**
- Production-like environment
- Webhooks work as expected
- Tests webhook configuration
- No special setup needed

**Cons:**
- Slower iteration (need to rebuild/reload image)
- Requires Kind cluster

#### Option 2: Disable Webhooks for Local Dev (Not Recommended)

Add a flag to skip webhook setup:

```go
// In cmd/main.go
var disableWebhooks bool
flag.BoolVar(&disableWebhooks, "disable-webhooks", false, "Disable webhook server (for local development)")

// Then conditionally create webhook server
var webhookServer webhook.Server
if !disableWebhooks {
    webhookServer = webhook.NewServer(webhook.Options{TLSOpts: tlsOpts})
}

mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
    // ...
    WebhookServer: webhookServer,
})
```

**Pros:**
- Fast iteration with `make run`
- No cluster required

**Cons:**
- Webhooks are not tested
- Validation/defaulting logic is skipped
- Not production-representative
- Can miss issues that only appear with webhooks

#### Option 3: Generate Self-Signed Certificates

Generate certificates locally before running:

```bash
# Create certificate directory
CERT_DIR=$(mktemp -d)

# Generate CA and serving certificate
openssl req -x509 -newkey rsa:4096 -nodes \
  -keyout "$CERT_DIR/tls.key" \
  -out "$CERT_DIR/tls.crt" \
  -days 365 -subj "/CN=webhook-service.caapkw-system.svc"

# Run controller with custom cert directory
go run ./cmd/main.go --cert-dir="$CERT_DIR"
```

**Pros:**
- Webhooks are functional
- Fast iteration

**Cons:**
- Manual certificate generation
- CA bundle not injected (webhooks won't actually be called by API server)
- Only tests webhook server startup, not actual webhook flow

#### Option 4: Use envtest with Webhooks

Use controller-runtime's envtest which can run webhooks:

```go
// In test files
testEnv = &envtest.Environment{
    CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
    WebhookInstallOptions: envtest.WebhookInstallOptions{
        Paths: []string{filepath.Join("..", "config", "webhook")},
    },
}

cfg, err := testEnv.Start()
// webhooks will be available at the test API server
```

**Pros:**
- Webhooks fully functional in tests
- No external cluster needed
- Good for integration tests

**Cons:**
- Only works in test context
- Not suitable for manual testing/debugging

## Recommendation

For CAAPKW development:

1. **Primary workflow**: Deploy to Kind cluster (Option 1)
   - Use `scripts/demo.sh` for complete workflow testing
   - Use `scripts/local-dev-setup.sh` to create the environment

2. **Quick iterations**: Use envtest in integration tests (Option 4)
   - Write comprehensive webhook tests
   - Run with `make test`

3. **Avoid**: Disabling webhooks or running `make run` directly
   - Webhooks are a core part of the API
   - Skipping them can hide bugs

## Troubleshooting

### Error: "open .../serving-certs/tls.crt: no such file or directory"

This means the controller is trying to start a webhook server but can't find the TLS certificates.

**Solution**: Deploy to a cluster with cert-manager instead of running locally with `go run`.

### Error: "x509: certificate signed by unknown authority"

The API server doesn't trust the webhook's certificate authority.

**Solution**: Ensure cert-manager has injected the CA bundle into the webhook configuration. Check:

```bash
kubectl get validatingwebhookconfigurations.admissionregistration.k8s.io -o yaml
kubectl get mutatingwebhookconfigurations.admissionregistration.k8s.io -o yaml
```

Look for the `caBundle` field - it should be populated.

### Webhooks Not Being Called

Check that webhooks are properly registered:

```bash
kubectl get validatingwebhookconfigurations
kubectl get mutatingwebhookconfigurations
```

Check controller logs for webhook server startup:

```bash
kubectl logs -n caapkw-system deployment/caapkw-controller-manager
```

## References

- [cert-manager Documentation](https://cert-manager.io/docs/)
- [Kubebuilder Webhook Guide](https://book.kubebuilder.io/cronjob-tutorial/webhook-implementation.html)
- [controller-runtime Webhook Documentation](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/webhook)
