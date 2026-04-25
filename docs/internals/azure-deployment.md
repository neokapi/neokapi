# Azure Deployment

## Architecture overview

The deployment creates a full Bowrain stack on Azure Container Apps:

| Component  | Azure resource             | Domain (prod)        | Domain (dev)             |
| ---------- | -------------------------- | -------------------- | ------------------------ |
| API server | Container App              | `bowrain.cloud`      | `dev.bowrain.cloud`      |
| Worker     | Container App              | (no ingress)         | (no ingress)             |
| Keycloak   | Container App              | `auth.bowrain.cloud` | `auth.dev.bowrain.cloud` |
| Database   | PostgreSQL Flexible Server | private network      | private network          |
| Cache      | Azure Cache for Redis      | private network      | private network          |
| Messaging  | Azure Service Bus          | managed identity     | managed identity         |
| Secrets    | Azure Key Vault            | RBAC                 | RBAC                     |
| Images     | Azure Container Registry   | managed identity     | managed identity         |

Infrastructure is defined in Bicep in the [bowrain-infra](https://github.com/neokapi/bowrain-infra) repo and deployed via GitHub Actions.

## Workflow structure

The `deploy-azure.yml` workflow runs three jobs in sequence:

```
lookup  →  build  →  deploy-apps
```

1. **lookup** — reads infrastructure values from GitHub environment variables (set by bowrain-infra's sync script) and computes the image tag
2. **build** — builds Docker images and pushes them to ACR
3. **deploy-apps** — checks out bowrain-infra repo, deploys Container Apps and DNS records via `apps.bicep`, using infra values from environment variables and the newly built image tag

Core infrastructure (identity, networking, databases, caches, ACR, Key Vault) is managed entirely in the [bowrain-infra](https://github.com/neokapi/bowrain-infra) repo. After deploying core infrastructure, bowrain-infra runs `sync-vars.sh` to push outputs to this repo's GitHub environment variables.

### Triggers

- **Push to `main`** on paths `bowrain/`, `core/`, `platform/`, `docker/` → deploys to `dev`
- **Manual dispatch** (`workflow_dispatch`) → choose `dev` or `prod`

## Pre-deployment setup

### 1. Azure subscription

Note your subscription and tenant IDs:

```bash
az account show --query '{subscriptionId:id, tenantId:tenantId}' -o table
```

### 2. Resource groups

Create resource groups for each environment and for DNS:

```bash
az group create --name rg-bowrain-d-weu --location northeurope
az group create --name rg-bowrain-p-weu --location northeurope
az group create --name bowrain-dns-rg   --location northeurope
```

### 3. DNS zone

Create the `bowrain.cloud` zone and point your domain registrar's NS records to Azure:

```bash
az network dns zone create -g bowrain-dns-rg -n bowrain.cloud

# Get the nameservers to configure at your registrar
az network dns zone show -g bowrain-dns-rg -n bowrain.cloud --query nameServers -o tsv
```

Update your domain registrar's NS records to the values returned above.

#### Apex domain (prod)

The prod API uses the zone apex (`bowrain.cloud`). DNS CNAME records cannot be created at the zone apex, so Bicep only creates the TXT validation record for prod. You have two options to route traffic to the apex:

**Option A — Registrar ALIAS/ANAME:** If your registrar supports ALIAS or ANAME records, point `bowrain.cloud` to the Container App FQDN shown in the deployment output.

**Option B — Azure Front Door:** Create an Azure Front Door profile with `bowrain.cloud` as the custom domain and the API Container App as the origin. This also gives you CDN, WAF, and global load balancing.

Dev subdomains (`dev.bowrain.cloud`, `auth.dev.bowrain.cloud`) use standard CNAME records created automatically by Bicep.

### 4. Service principal with OIDC federation

Create an app registration with federated credentials for keyless GitHub Actions authentication:

```bash
# Create app registration
az ad app create --display-name github-bowrain-deployer
APP_ID=$(az ad app list --display-name github-bowrain-deployer --query '[0].appId' -o tsv)

# Create service principal
az ad sp create --id "$APP_ID"

# Grant Contributor on the subscription
SUBSCRIPTION_ID=$(az account show --query id -o tsv)
az role assignment create \
  --assignee "$APP_ID" \
  --role Contributor \
  --scope "/subscriptions/${SUBSCRIPTION_ID}"

# Add federated credentials for GitHub Actions
# One for main branch pushes, one per environment
for CRED in \
  '{"name":"github-main","subject":"repo:YOUR_ORG/YOUR_REPO:ref:refs/heads/main"}' \
  '{"name":"github-env-dev","subject":"repo:YOUR_ORG/YOUR_REPO:environment:dev"}' \
  '{"name":"github-env-prod","subject":"repo:YOUR_ORG/YOUR_REPO:environment:prod"}'; do
  echo "$CRED" | jq '. + {"issuer":"https://token.actions.githubusercontent.com","audiences":["api://AzureADTokenExchange"]}' \
    | az ad app federated-credential create --id "$APP_ID" --parameters @-
done
```

Replace `YOUR_ORG/YOUR_REPO` with your actual GitHub repository path.

### 5. Generate application secrets

```bash
# JWT signing secret
openssl rand -base64 32

# OIDC client secret (Keycloak ↔ bowrain-server)
openssl rand -base64 32

# Keycloak admin password
openssl rand -base64 24

# PostgreSQL admin credentials
# Choose a username (e.g. "sqladmin") and generate a password:
openssl rand -base64 24
```

### 6. GitHub repository secrets

Go to **Settings → Secrets and variables → Actions** and create these repository secrets:

| Secret                    | Value                                                 |
| ------------------------- | ----------------------------------------------------- |
| `AZURE_CLIENT_ID`         | App ID from step 4                                    |
| `AZURE_TENANT_ID`         | Azure tenant ID                                       |
| `AZURE_SUBSCRIPTION_ID`   | Azure subscription ID                                 |
| `POSTGRES_ADMIN_LOGIN`    | PostgreSQL admin username (e.g. `sqladmin`)           |
| `POSTGRES_ADMIN_PASSWORD` | Generated PostgreSQL password                         |
| `JWT_SECRET`              | Generated base64 JWT secret                           |
| `OIDC_CLIENT_SECRET`      | Generated base64 OIDC secret                          |
| `KEYCLOAK_ADMIN_PASSWORD` | Generated Keycloak password                           |
| `GH_PAT`                  | GitHub PAT with permission to read bowrain-infra repo |

### 7. GitHub environments

Go to **Settings → Environments** and create:

- **dev** — no protection rules (auto-deploys on push to main)
- **prod** — add required reviewers for approval before deploy

## First deployment

With the two-repo split, the first deployment works cleanly:

1. Deploy core infrastructure from bowrain-infra repo (creates ACR, databases, etc.)
2. Run `sync-vars.sh` from bowrain-infra to push outputs to this repo's GitHub environment
3. Trigger the deploy workflow here — **`lookup`** reads infra values, **`build`** pushes images, **`deploy-apps`** creates Container Apps

No bootstrapping workaround is needed. All three jobs succeed on first deploy.

### Verify the deployment

```bash
# Check Container App status
az containerapp show \
  --name ca-bowrain-api-d-weu \
  --resource-group rg-bowrain-d-weu \
  --query '{status:properties.runningStatus, fqdn:properties.configuration.ingress.fqdn}' \
  -o table

# Check logs
az containerapp logs show \
  --name ca-bowrain-api-d-weu \
  --resource-group rg-bowrain-d-weu \
  --follow
```

## Environment configuration

Bicep parameters live in the [bowrain-infra](https://github.com/neokapi/bowrain-infra) repo under `environments/{dev,prod}/`:

| File                                | Layer | Environment | Notes                                        |
| ----------------------------------- | ----- | ----------- | -------------------------------------------- |
| `environments/dev/core.bicepparam`  | Core  | dev         | Identity, networking, databases, caches, ACR |
| `environments/prod/core.bicepparam` | Core  | prod        | Identity, networking, databases, caches, ACR |
| `environments/dev/apps.bicepparam`  | Apps  | dev         | Container Apps, DNS                          |
| `environments/prod/apps.bicepparam` | Apps  | prod        | Container Apps, DNS                          |

Sensitive parameters (database credentials, JWT secret, etc.) are passed via GitHub secrets at deploy time and never stored in parameter files. Core infrastructure outputs (managed identity ID, ACR login server, etc.) are synced to this repo's GitHub environment variables by bowrain-infra's `sync-vars.sh` script.

## Custom domains

| Environment | API                 | Auth                     |
| ----------- | ------------------- | ------------------------ |
| **prod**    | `bowrain.cloud`     | `auth.bowrain.cloud`     |
| **dev**     | `dev.bowrain.cloud` | `auth.dev.bowrain.cloud` |

TLS certificates are automatically provisioned and managed by Azure Container Apps.

## Infrastructure modules

All Bicep modules live in the [bowrain-infra](https://github.com/neokapi/bowrain-infra) repo under `modules/`. The two orchestrator files (`core.bicep` and `apps.bicep`) compose these modules into deployment layers:

| Module                        | Layer | Resources                                               |
| ----------------------------- | ----- | ------------------------------------------------------- |
| `identity.bicep`              | Core  | User-assigned managed identity, RBAC role assignments   |
| `network.bicep`               | Core  | VNet, subnets, private DNS zones                        |
| `acr.bicep`                   | Core  | Container Registry, AcrPull role assignment             |
| `keyvault.bicep`              | Core  | Key Vault, secrets (JWT, OIDC, Redis key)               |
| `postgres.bicep`              | Core  | PostgreSQL Flexible Server, PgBouncer config, databases |
| `redis.bicep`                 | Core  | Redis Cache, private endpoint                           |
| `servicebus.bicep`            | Core  | Service Bus namespace                                   |
| `containerapp-env.bicep`      | Core  | Container Apps Environment, Log Analytics               |
| `containerapp-api.bicep`      | Apps  | API server Container App                                |
| `containerapp-worker.bicep`   | Apps  | Worker Container App (KEDA Service Bus scaler)          |
| `containerapp-keycloak.bicep` | Apps  | Keycloak Container App                                  |
| `dns.bicep`                   | Apps  | CNAME and TXT records in bowrain.cloud zone             |

## Linting

```bash
make gha-lint     # Lint GitHub Actions workflows (requires actionlint)
```

Bicep linting is handled in the [bowrain-infra](https://github.com/neokapi/bowrain-infra) repo (`make lint`).
