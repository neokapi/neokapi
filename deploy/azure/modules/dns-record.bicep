// Helper module for creating a CNAME record in an existing DNS zone.
// Deployed into the DNS zone's resource group via cross-RG scope.

param dnsZoneName string
param recordName string
param targetFqdn string

resource dnsZone 'Microsoft.Network/dnsZones@2023-07-01-preview' existing = {
  name: dnsZoneName
}

resource cnameRecord 'Microsoft.Network/dnsZones/CNAME@2023-07-01-preview' = {
  parent: dnsZone
  name: recordName
  properties: {
    TTL: 3600
    CNAMERecord: {
      cname: targetFqdn
    }
  }
}
