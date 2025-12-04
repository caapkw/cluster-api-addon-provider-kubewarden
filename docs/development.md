## Development

### Running the Controller

CAAPKW uses admission webhooks which require TLS certificates. For local development, there are two approaches:

1. **Recommended: Deploy to Kind cluster** (with cert-manager handling certificates automatically)
   ```bash
   ./scripts/local-dev-setup.sh  # Creates environment
   ./scripts/demo.sh              # Deploys controller and runs demo
   ```

2. **Not Recommended: Run locally with `make run`** - This will fail because webhook certificates are not available outside a cluster. See [docs/webhook-certificates.md](./webhook-certificates.md) for details.

For more information on webhook certificate management, see [Webhook Certificates Documentation](./webhook-certificates.md).
