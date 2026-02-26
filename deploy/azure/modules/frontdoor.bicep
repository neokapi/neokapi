// Azure Front Door Standard — reverse proxy for API + static web UI.
// Routes /api/* to the Container App backend and /* to the Storage Account static website.

param prefix string
param tags object
param customDomain string
param apiBackendFqdn string
param webBackendHostname string
param dnsZoneName string
param dnsZoneResourceGroup string

// ── Front Door profile ───────────────────────────────────────────────────────

resource frontDoorProfile 'Microsoft.Cdn/profiles@2024-02-02' = {
  name: '${prefix}-fd'
  location: 'global'
  tags: tags
  sku: {
    name: 'Standard_AzureFrontDoor'
  }
}

// ── Endpoint ─────────────────────────────────────────────────────────────────

resource endpoint 'Microsoft.Cdn/profiles/afdEndpoints@2024-02-02' = {
  parent: frontDoorProfile
  name: '${prefix}-endpoint'
  location: 'global'
  tags: tags
  properties: {
    enabledState: 'Enabled'
  }
}

// ── Origin Groups ────────────────────────────────────────────────────────────

resource apiOriginGroup 'Microsoft.Cdn/profiles/originGroups@2024-02-02' = {
  parent: frontDoorProfile
  name: 'api-origin-group'
  properties: {
    loadBalancingSettings: {
      sampleSize: 4
      successfulSamplesRequired: 3
    }
    healthProbeSettings: {
      probePath: '/health'
      probeRequestType: 'HEAD'
      probeProtocol: 'Http'
      probeIntervalInSeconds: 30
    }
  }
}

resource webOriginGroup 'Microsoft.Cdn/profiles/originGroups@2024-02-02' = {
  parent: frontDoorProfile
  name: 'web-origin-group'
  properties: {
    loadBalancingSettings: {
      sampleSize: 4
      successfulSamplesRequired: 3
    }
    healthProbeSettings: {
      probePath: '/index.html'
      probeRequestType: 'HEAD'
      probeProtocol: 'Https'
      probeIntervalInSeconds: 60
    }
  }
}

// ── Origins ──────────────────────────────────────────────────────────────────

resource apiOrigin 'Microsoft.Cdn/profiles/originGroups/origins@2024-02-02' = {
  parent: apiOriginGroup
  name: 'api-origin'
  properties: {
    hostName: apiBackendFqdn
    httpPort: 80
    httpsPort: 443
    originHostHeader: apiBackendFqdn
    priority: 1
    weight: 1000
  }
}

resource webOrigin 'Microsoft.Cdn/profiles/originGroups/origins@2024-02-02' = {
  parent: webOriginGroup
  name: 'web-origin'
  properties: {
    hostName: webBackendHostname
    httpPort: 80
    httpsPort: 443
    originHostHeader: webBackendHostname
    priority: 1
    weight: 1000
  }
}

// ── Custom Domain ────────────────────────────────────────────────────────────

resource customDomainResource 'Microsoft.Cdn/profiles/customDomains@2024-02-02' = {
  parent: frontDoorProfile
  name: replace(customDomain, '.', '-')
  properties: {
    hostName: customDomain
    tlsSettings: {
      certificateType: 'ManagedCertificate'
      minimumTlsVersion: 'TLS12'
    }
  }
}

// DNS TXT record for Front Door domain validation (_dnsauth.<domain>).
module dnsAuthTxt 'dns-txt-record.bicep' = {
  name: 'dns-fd-auth-txt'
  scope: resourceGroup(dnsZoneResourceGroup)
  params: {
    dnsZoneName: dnsZoneName
    recordName: '_dnsauth.${customDomain}'
    txtValue: customDomainResource.properties.validationProperties.validationToken
  }
}

// ── Routes ───────────────────────────────────────────────────────────────────

resource apiRoute 'Microsoft.Cdn/profiles/afdEndpoints/routes@2024-02-02' = {
  parent: endpoint
  name: 'api-route'
  properties: {
    customDomains: [
      {
        id: customDomainResource.id
      }
    ]
    originGroup: {
      id: apiOriginGroup.id
    }
    patternsToMatch: [
      '/api/*'
    ]
    forwardingProtocol: 'HttpsOnly'
    httpsRedirect: 'Enabled'
    linkToDefaultDomain: 'Enabled'
    cacheConfiguration: null
  }
  dependsOn: [
    apiOrigin
  ]
}

resource webRoute 'Microsoft.Cdn/profiles/afdEndpoints/routes@2024-02-02' = {
  parent: endpoint
  name: 'web-route'
  properties: {
    customDomains: [
      {
        id: customDomainResource.id
      }
    ]
    originGroup: {
      id: webOriginGroup.id
    }
    patternsToMatch: [
      '/*'
    ]
    forwardingProtocol: 'HttpsOnly'
    httpsRedirect: 'Enabled'
    linkToDefaultDomain: 'Enabled'
    cacheConfiguration: {
      queryStringCachingBehavior: 'IgnoreQueryString'
      compressionSettings: {
        isCompressionEnabled: true
        contentTypesToCompress: [
          'text/html'
          'text/css'
          'application/javascript'
          'application/json'
          'image/svg+xml'
        ]
      }
    }
  }
  dependsOn: [
    webOrigin
    apiRoute
  ]
}

// ── Outputs ──────────────────────────────────────────────────────────────────

output frontDoorFqdn string = endpoint.properties.hostName
output frontDoorId string = frontDoorProfile.id
