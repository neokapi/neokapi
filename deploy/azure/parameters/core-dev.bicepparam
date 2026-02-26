using '../core.bicep'

param environment = 'dev'
param location = 'northeurope'

// Sensitive parameters must be provided via CLI or environment:
//   --parameters postgresAdminLogin=<value>
//   --parameters postgresAdminPassword=<value>
//   --parameters jwtSecret=<value>
//   --parameters oidcClientSecret=<value>
