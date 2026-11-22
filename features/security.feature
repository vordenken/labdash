Feature: Hardened defaults for a public deployment
  Scenario: A forged X-Forwarded-For header does not reset the login limit
    Given LABDASH_TRUSTED_PROXIES is not set
    And a client has failed to sign in three times
    When the client signs in with the correct password and a new X-Forwarded-For address
    Then the response is "429 Too Many Requests"

  Scenario: The real client address is used behind a trusted proxy
    Given LABDASH_TRUSTED_PROXIES contains the reverse proxy's network
    When a client signs in through the proxy
    Then the login log entry shows the client address from X-Forwarded-For

  Scenario: A page on a sibling subdomain cannot change the dashboard
    Given I am signed in to the dashboard at "dash.example.org"
    When a page on "evil.example.org" sends a request to save the configuration
    Then the request is rejected with "403 Forbidden"
    And the configuration is unchanged

  Scenario: Script links are never rendered
    Given a container label sets "labdash.href" to "javascript:alert(1)"
    When the dashboard shows the discovered tile
    Then the tile has no link
    And saving a configuration with that link is rejected with an explanatory error

  Scenario: The browser is limited to the dashboard's own origin
    When I open the dashboard
    Then every response carries a Content-Security-Policy that only allows the dashboard's own origin
    And icon files are served with a sandboxing Content-Security-Policy

  Scenario: Icon downloads cannot reach arbitrary hosts
    Given icon downloads are enabled
    When a tile uses an icon name that is not a known collection entry
    Then the server only requests fixed jsDelivr URLs for Dashboard Icons or Material Design Icons
    And a response larger than 2 MiB or not shaped like an image is discarded
    And an unknown icon is not requested again for 24 hours
    And a redirect to another host is not followed

  Scenario: A mistyped switch stops startup instead of using its default
    Given LABDASH_ICON_DOWNLOAD is set to "nein"
    When labdash starts
    Then it exits with an error naming LABDASH_ICON_DOWNLOAD
    And no outbound request is made
