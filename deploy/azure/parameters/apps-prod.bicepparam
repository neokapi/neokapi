using '../apps.bicep'

param environment = 'prod'
param location = 'northeurope'
param dnsZoneResourceGroup = 'bowrain-dns-rg'
param imageTag = 'latest'

param tags = {
  project: 'bowrain'
  environment: 'prod'
  managedBy: 'bicep'
}

// Core infrastructure outputs must be provided via CLI:
//   --parameters managedIdentityId=<value>
//   --parameters acrLoginServer=<value>
//   --parameters containerAppEnvId=<value>
//   --parameters postgresFqdn=<value>
//   --parameters redisHostname=<value>
//   --parameters serviceBusConnectionString=<value>
//   --parameters keyVaultUri=<value>
//
// Sensitive parameters must be provided via CLI:
//   --parameters postgresAdminLogin=<value>
//   --parameters postgresAdminPassword=<value>
//   --parameters keycloakAdminPassword=<value>
