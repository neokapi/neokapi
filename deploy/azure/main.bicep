// Bowrain Azure Infrastructure
// Orchestrator that deploys all modules for the Bowrain localization platform.

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

@description('Name of the existing DNS zone for samarb.ai')
param dnsZoneName string = 'samarb.ai'

@description('Resource group containing the DNS zone')
param dnsZoneResourceGroup string

@description('Container image tag for bowrain-server')
param imageTag string

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

@description('Keycloak admin password')
@secure()
param keycloakAdminPassword string

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

// ── DNS ──────────────────────────────────────────────────────────────────────

module dns 'modules/dns.bicep' = {
  name: '${prefix}-dns'
  params: {
    dnsZoneName: dnsZoneName
    dnsZoneResourceGroup: dnsZoneResourceGroup
    environment: environment
    apiAppFqdn: containerAppApi.outputs.fqdn
    keycloakAppFqdn: containerAppKeycloak.outputs.fqdn
  }
}

// ── Container App: API ───────────────────────────────────────────────────────

module containerAppApi 'modules/containerapp-api.bicep' = {
  name: '${prefix}-app-api'
  params: {
    prefix: prefix
    location: location
    tags: tags
    containerAppEnvId: containerAppEnv.outputs.id
    managedIdentityId: identity.outputs.id
    acrLoginServer: acr.outputs.loginServer
    imageTag: imageTag
    environment: environment
    postgresHost: postgres.outputs.fqdn
    postgresDbName: 'bowrain'
    redisHost: redis.outputs.hostname
    serviceBusConnectionString: servicebus.outputs.connectionString
    keyVaultUri: keyvault.outputs.uri
    keycloakIssuerUrl: 'https://auth.bowrain.samarb.ai/realms/bowrain'
    customDomain: environment == 'prod' ? 'bowrain.samarb.ai' : 'bowrain-dev.samarb.ai'
  }
}

// ── Container App: Worker ────────────────────────────────────────────────────

module containerAppWorker 'modules/containerapp-worker.bicep' = {
  name: '${prefix}-app-worker'
  params: {
    prefix: prefix
    location: location
    tags: tags
    containerAppEnvId: containerAppEnv.outputs.id
    managedIdentityId: identity.outputs.id
    acrLoginServer: acr.outputs.loginServer
    imageTag: imageTag
    environment: environment
    postgresHost: postgres.outputs.fqdn
    postgresDbName: 'bowrain'
    redisHost: redis.outputs.hostname
    serviceBusConnectionString: servicebus.outputs.connectionString
    keyVaultUri: keyvault.outputs.uri
  }
}

// ── Container App: Keycloak ──────────────────────────────────────────────────

module containerAppKeycloak 'modules/containerapp-keycloak.bicep' = {
  name: '${prefix}-app-keycloak'
  params: {
    prefix: prefix
    location: location
    tags: tags
    containerAppEnvId: containerAppEnv.outputs.id
    managedIdentityId: identity.outputs.id
    acrLoginServer: acr.outputs.loginServer
    imageTag: imageTag
    environment: environment
    postgresHost: postgres.outputs.fqdn
    postgresDbName: 'keycloak'
    postgresAdminLogin: postgresAdminLogin
    postgresAdminPassword: postgresAdminPassword
    keycloakAdminPassword: keycloakAdminPassword
    customDomain: environment == 'prod' ? 'auth.bowrain.samarb.ai' : 'auth.bowrain-dev.samarb.ai'
  }
}

// ── Outputs ──────────────────────────────────────────────────────────────────

output apiUrl string = 'https://${containerAppApi.outputs.fqdn}'
output keycloakUrl string = 'https://${containerAppKeycloak.outputs.fqdn}'
output acrLoginServer string = acr.outputs.loginServer
output keyVaultUri string = keyvault.outputs.uri
