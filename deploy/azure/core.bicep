// Bowrain Azure Infrastructure — Core Layer
// Deploys long-lived infrastructure: identity, networking, databases, caches, and registries.
// Outputs are consumed by the apps layer (apps.bicep) via the deployment workflow.

targetScope = 'resourceGroup'

@description('Deployment environment')
@allowed(['dev', 'prod'])
param environment string

@description('Azure region for all resources')
param location string = resourceGroup().location

@description('Tags applied to all resources')
param tags object = {
  project: 'bowrain'
  environment: environment
}

@description('PostgreSQL administrator login')
@secure()
param postgresAdminLogin string

@description('PostgreSQL administrator password')
@secure()
param postgresAdminPassword string

@description('JWT signing secret for bowrain-server')
@secure()
param jwtSecret string

@description('OIDC client secret for Keycloak')
@secure()
param oidcClientSecret string

// ── Naming convention ────────────────────────────────────────────────────────

var prefix = 'bowrain-${environment}'

// ── Identity ─────────────────────────────────────────────────────────────────

module identity 'modules/identity.bicep' = {
  name: '${prefix}-identity'
  params: {
    prefix: prefix
    location: location
    tags: tags
  }
}

// ── Network ──────────────────────────────────────────────────────────────────

module network 'modules/network.bicep' = {
  name: '${prefix}-network'
  params: {
    prefix: prefix
    location: location
    tags: tags
  }
}

// ── Container Registry ───────────────────────────────────────────────────────

module acr 'modules/acr.bicep' = {
  name: '${prefix}-acr'
  params: {
    prefix: prefix
    location: location
    tags: tags
    principalId: identity.outputs.principalId
  }
}

// ── Redis ────────────────────────────────────────────────────────────────────

module redis 'modules/redis.bicep' = {
  name: '${prefix}-redis'
  params: {
    prefix: prefix
    location: location
    tags: tags
    subnetId: network.outputs.dataServicesSubnetId
    privateDnsZoneId: network.outputs.redisPrivateDnsZoneId
    environment: environment
  }
}

// ── Key Vault ────────────────────────────────────────────────────────────────

module keyvault 'modules/keyvault.bicep' = {
  name: '${prefix}-keyvault'
  params: {
    prefix: prefix
    location: location
    tags: tags
    principalId: identity.outputs.principalId
    jwtSecret: jwtSecret
    oidcClientSecret: oidcClientSecret
    redisAccessKey: redis.outputs.primaryKey
  }
}

// ── PostgreSQL ───────────────────────────────────────────────────────────────

module postgres 'modules/postgres.bicep' = {
  name: '${prefix}-postgres'
  params: {
    prefix: prefix
    location: location
    tags: tags
    subnetId: network.outputs.dataServicesSubnetId
    privateDnsZoneId: network.outputs.postgresPrivateDnsZoneId
    administratorLogin: postgresAdminLogin
    administratorPassword: postgresAdminPassword
    managedIdentityPrincipalId: identity.outputs.principalId
    environment: environment
  }
}

// ── Service Bus ──────────────────────────────────────────────────────────────

module servicebus 'modules/servicebus.bicep' = {
  name: '${prefix}-servicebus'
  params: {
    prefix: prefix
    location: location
    tags: tags
  }
}

// ── Container Apps Environment ───────────────────────────────────────────────

module containerAppEnv 'modules/containerapp-env.bicep' = {
  name: '${prefix}-cae'
  params: {
    prefix: prefix
    location: location
    tags: tags
    subnetId: network.outputs.containerAppsSubnetId
  }
}

// ── Storage (Web UI) ──────────────────────────────────────────────────────────

module storageWeb 'modules/storage-web.bicep' = {
  name: '${prefix}-storage-web'
  params: {
    prefix: prefix
    location: location
    tags: tags
    managedIdentityId: identity.outputs.id
    managedIdentityPrincipalId: identity.outputs.principalId
  }
}

// ── Outputs (consumed by apps layer) ─────────────────────────────────────────

output managedIdentityId string = identity.outputs.id
output acrLoginServer string = acr.outputs.loginServer
output containerAppEnvId string = containerAppEnv.outputs.id
output postgresFqdn string = postgres.outputs.fqdn
output redisHostname string = redis.outputs.hostname
output serviceBusConnectionString string = servicebus.outputs.connectionString
output keyVaultUri string = keyvault.outputs.uri
output storageAccountName string = storageWeb.outputs.storageAccountName
output webHostname string = storageWeb.outputs.webHostname
