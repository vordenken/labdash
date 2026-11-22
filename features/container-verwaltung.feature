# language: de
Funktionalität: Problem-Container in der Container-Verwaltung öffnen
  Als Betreiber eines Homelabs
  möchte ich von einem Container mit Problem direkt in meine Container-Verwaltung (z. B. Arcane) springen,
  damit ich Logs und Ursache schnell prüfen kann.

  Grundlage:
    Angenommen LABDASH_CONTAINER_LINK ist "https://arcane.example.org/containers/{id}?env=0"
    Und ich bin angemeldet

  Szenario: Gestoppter Container zeigt den Verwaltungs-Link auf der Kachel
    Angenommen der Container "wiki" mit der ID "bbb222" ist gestoppt
    Wenn ich das Dashboard öffne
    Dann zeigt die Kachel "Wiki" eine Schaltfläche "Wiki in der Container-Verwaltung öffnen"
    Und ein Klick darauf öffnet "https://arcane.example.org/containers/bbb222?env=0" in einem neuen Tab
    Und die Kachel-Adresse selbst wird dabei nicht geöffnet

  Szenario: Verwaltungs-Link im Container-Status-Dialog
    Angenommen der Container "wiki" ist gestoppt
    Wenn ich im Kopfbereich die gestoppten Container öffne
    Dann hat die Zeile "wiki" einen Link in die Container-Verwaltung

  Szenariogrundriss: Nur Container mit Problem erhalten einen Link
    Angenommen ein Container ist "<Zustand>"
    Dann wird ein Verwaltungs-Link <Anzeige>

    Beispiele:
      | Zustand                 | Anzeige          |
      | gestoppt                | angezeigt        |
      | unhealthy               | angezeigt        |
      | neu startend            | angezeigt        |
      | pausiert                | angezeigt        |
      | läuft                   | nicht angezeigt  |
      | schläft (Sablier)       | nicht angezeigt  |

  Szenario: Ohne Anmeldung bleibt die Adresse der Verwaltung verborgen
    Angenommen ich bin nicht angemeldet
    Wenn die Anmeldeseite die Sitzungsdaten lädt
    Dann enthalten sie keinen Verwaltungs-Link

  Szenario: Ungültige Vorlage verhindert den Start
    Angenommen LABDASH_CONTAINER_LINK ist "arcane/containers/{id}"
    Wenn labdash startet
    Dann bricht der Start mit einer Fehlermeldung zu LABDASH_CONTAINER_LINK ab
