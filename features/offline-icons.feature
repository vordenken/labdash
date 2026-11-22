# language: de
Funktionalität: Kachel-Icons auch ohne Internetzugang anzeigen

  Szenario: Ein bekanntes Icon wird einmal geladen und danach lokal angezeigt
    Angenommen der Icon-Download ist eingeschaltet
    Und eine Kachel hat das Icon "jellyfin"
    Wenn ich das Dashboard zum ersten Mal öffne
    Dann lädt der labdash-Server das Icon einmal aus „Dashboard Icons“ und speichert es
    Und der Browser stellt keine Anfrage an einen anderen Server
    Wenn die Internetverbindung danach ausfällt
    Dann zeigt die Kachel das Icon weiterhin an

  Szenario: Eigene Bilder haben Vorrang
    Angenommen im Icon-Ordner liegt die Datei "jellyfin.png"
    Und eine Kachel hat das Icon "jellyfin"
    Wenn ich das Dashboard öffne
    Dann zeigt die Kachel das Bild aus "jellyfin.png"
    Und es wird nichts heruntergeladen

  Szenario: Den Icon-Download abschalten
    Angenommen der Icon-Download ist mit LABDASH_ICON_DOWNLOAD=false ausgeschaltet
    Und das Icon "plex" wurde früher schon heruntergeladen
    Und eine andere Kachel hat das Icon "grafana", das noch nie geladen wurde
    Wenn ich das Dashboard öffne
    Dann zeigt die Kachel mit "plex" weiterhin ihr Icon
    Und die Kachel mit "grafana" zeigt Initialen
    Und labdash stellt keine Anfrage ins Internet

  Szenario: Unbekannte oder externe Icons durch Initialen ersetzen
    Angenommen eine Kachel "Media Library" hat das Icon "https://cdn.example.org/media.png"
    Und eine andere Kachel "Photo Archive" hat das Icon "gibt-es-nicht"
    Wenn ich das Dashboard öffne
    Dann zeigen die Kacheln die Initialen "ML" und "PA"
    Und niemand fragt "cdn.example.org" an
