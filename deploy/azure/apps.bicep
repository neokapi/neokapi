// Bowrain Azure Infrastructure — Apps Layer
// Deploys Container Apps and DNS records.
// Depends on core infrastructure outputs passed as parameters.

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

@description('Name of the existing DNS zone for bowrain.cloud')
param dnsZoneName string = 'bowrain.cloud'

@description('Resource group containing the DNS zone')
param dnsZoneResourceGroup string

@description('Container image tag')
param imageTag string

@description('PostgreSQL administrator login')
@secure()
param postgresAdminLogin string

@description('PostgreSQL administrator password')
@secure()
param postgresAdminPassword string

@description('Keycloak admin password')
@secure()
param keycloakAdminPassword string

// ── Core infrastructure outputs ──────────────────────────────────────────────

@description('Resource ID of the user-assigned managed identity')
param managedIdentityId string

@description('ACR login server hostname')
param acrLoginServer string

@description('Container Apps Environment resource ID')
param containerAppEnvId string

@description('PostgreSQL server FQDN')
param postgresFqdn string

@description('Redis cache hostname')
param redisHostname string

@description('Service Bus connection string')
@secure()
param serviceBusConnectionString string

@description('Key Vault URI')
param keyVaultUri string

@description('Storage Account static website hostname (from core layer)')
param storageWebHostname string

@description('Storage Account name (from core layer)')
param storageAccountName string

// ── Naming convention ────────────────────────────────────────────────────────

var prefix = 'bowrain-${environment}'

// ── Container App: API ───────────────────────────────────────────────────────

module containerAppApi 'modules/containerapp-api.bicep' = {
  name: '${prefix}-app-api'
  params: {
    prefix: prefix
    location: location
    tags: tags
    containerAppEnvId: containerAppEnvId
    managedIdentityId: managedIdentityId
    acrLoginServer: acrLoginServer
    imageTag: imageTag
    environment: environment
    postgresHost: postgresFqdn
    postgresDbName: 'bowrain'
    redisHost: redisHostname
    serviceBusConnectionString: serviceBusConnectionString
    keyVaultUri: keyVaultUri
    keycloakIssuerUrl: environment == 'prod' ? 'https://auth.bowrain.cloud/realms/bowrain' : 'https://auth.dev.bowrain.cloud/realms/bowrain'
  }
}

// ── Container App: Worker ────────────────────────────────────────────────────

module containerAppWorker 'modules/containerapp-worker.bicep' = {
  name: '${prefix}-app-worker'
  params: {
    prefix: prefix
    location: location
    tags: tags
    containerAppEnvId: containerAppEnvId
    managedIdentityId: managedIdentityId
    acrLoginServer: acrLoginServer
    imageTag: imageTag
    environment: environment
    postgresHost: postgresFqdn
    postgresDbName: 'bowrain'
    redisHost: redisHostname
    serviceBusConnectionString: serviceBusConnectionString
    keyVaultUri: keyVaultUri
  }
}

// ── Container App: Keycloak ──────────────────────────────────────────────────

module containerAppKeycloak 'modules/containerapp-keycloak.bicep' = {
  name: '${prefix}-app-keycloak'
  params: {
    prefix: prefix
    location: location
    tags: tags
    containerAppEnvId: containerAppEnvId
    managedIdentityId: managedIdentityId
    acrLoginServer: acrLoginServer
    imageTag: imageTag
    environment: environment
    postgresHost: postgresFqdn
    postgresDbName: 'keycloak'
    postgresAdminLogin: postgresAdminLogin
    postgresAdminPassword: postgresAdminPassword
    keycloakAdminPassword: keycloakAdminPassword
    customDomain: environment == 'prod' ? 'auth.bowrain.cloud' : 'auth.dev.bowrain.cloud'
  }
}

// ── Front Door ───────────────────────────────────────────────────────────────

module frontdoor 'modules/frontdoor.bicep' = {
  name: '${prefix}-frontdoor'
  params: {
    prefix: prefix
    tags: tags
    customDomain: environment == 'prod' ? 'bowrain.cloud' : 'dev.bowrain.cloud'
    apiBackendFqdn: containerAppApi.outputs.fqdn
    webBackendHostname: storageWebHostname
    dnsZoneName: dnsZoneName
    dnsZoneResourceGroup: dnsZoneResourceGroup
  }
}

// ── DNS ──────────────────────────────────────────────────────────────────────

module dns 'modules/dns.bicep' = {
  name: '${prefix}-dns'
  params: {
    dnsZoneName: dnsZoneName
    dnsZoneResourceGroup: dnsZoneResourceGroup
    environment: environment
    frontDoorFqdn: frontdoor.outputs.frontDoorFqdn
    keycloakAppFqdn: containerAppKeycloak.outputs.fqdn
  }
}

// ── Outputs ──────────────────────────────────────────────────────────────────

output apiUrl string = 'https://${containerAppApi.outputs.fqdn}'
output keycloakUrl string = 'https://${containerAppKeycloak.outputs.fqdn}'
output frontDoorFqdn string = frontdoor.outputs.frontDoorFqdn
output storageAccountName string = storageAccountName
