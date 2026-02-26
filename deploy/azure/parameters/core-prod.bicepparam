using '../core.bicep'

param environment = 'prod'
param location = 'northeurope'

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
