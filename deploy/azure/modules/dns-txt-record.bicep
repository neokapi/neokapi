// Helper module for creating a TXT record in an existing DNS zone.
// Used for custom domain validation on Container Apps.

param dnsZoneName string
param recordName string
param txtValue string

resource dnsZone 'Microsoft.Network/dnsZones@2023-07-01-preview' existing = {
  name: dnsZoneName
}

resource txtRecord 'Microsoft.Network/dnsZones/TXT@2023-07-01-preview' = {
  parent: dnsZone
  name: recordName
  properties: {
    TTL: 3600
    TXTRecords: [
      {
        value: [txtValue]
      }
    ]
  }
}
