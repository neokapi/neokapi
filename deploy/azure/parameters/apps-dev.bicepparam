using '../apps.bicep'

param environment = 'dev'
param location = 'northeurope'
param dnsZoneResourceGroup = 'bowrain-dns-rg'
param imageTag = 'latest'

// Core infrastructure outputs must be provided via CLI:
//   --parameters managedIdentityId=<value>
//   --parameters acrLoginServer=<value>
//   --parameters containerAppEnvId=<value>
//   --parameters postgresFqdn=<value>
//   --parameters redisHostname=<value>
//   --parameters serviceBusConnectionString=<value>
//   --parameters keyVaultUri=<value>
//   --parameters storageWebHostname=<value>
//   --parameters storageAccountName=<value>
//
// Sensitive parameters must be provided via CLI:
//   --parameters postgresAdminLogin=<value>
//   --parameters postgresAdminPassword=<value>
//   --parameters keycloakAdminPassword=<value>
