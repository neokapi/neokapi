using '../main.bicep'

param environment = 'dev'
param location = 'northeurope'
param dnsZoneResourceGroup = 'samarb-dns-rg'
param imageTag = 'latest'

// Sensitive parameters must be provided via CLI or environment:
//   --parameters postgresAdminLogin=<value>
//   --parameters postgresAdminPassword=<value>
//   --parameters jwtSecret=<value>
//   --parameters oidcClientSecret=<value>
//   --parameters keycloakAdminPassword=<value>
