using '../main.bicep'

param environment = 'prod'
param location = 'northeurope'
param dnsZoneResourceGroup = 'samarb-dns-rg'
param imageTag = 'latest'

param tags = {
  project: 'bowrain'
  environment: 'prod'
  managedBy: 'bicep'
}

// Sensitive parameters must be provided via CLI or environment:
//   --parameters postgresAdminLogin=<value>
//   --parameters postgresAdminPassword=<value>
//   --parameters jwtSecret=<value>
//   --parameters oidcClientSecret=<value>
//   --parameters keycloakAdminPassword=<value>
