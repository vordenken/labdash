# language: de
Funktionalität: Container- und GitHub-Releases automatisch veröffentlichen
  Szenario: Eine neue Version auf main veröffentlichen
    Angenommen die Version in semver.txt wurde auf main erhöht
    Wenn alle CI-Prüfungen erfolgreich sind
    Dann veröffentlicht die CI ein Multiarch-Image und den GitHub-Release mit passendem Tag
    Und die stabilen Image-Tags enthalten die Version, Major.Minor, Major und latest

  Szenario: Beta auf einem Feature-Branch veröffentlichen
    Angenommen eine Anwendung wurde auf einem feature-Branch geändert
    Wenn alle CI-Prüfungen erfolgreich sind
    Dann veröffentlicht die CI einen nummerierten Beta-Release und ein Beta-Image
    Aber die stabilen Image-Tags bleiben unverändert

  Szenario: Kein Release nur für Dokumentation oder Tests
    Angenommen nur Dokumentation oder Tests wurden geändert
    Und die Version auf main bleibt unverändert
    Wenn die CI erfolgreich ist
    Dann veröffentlicht sie keinen Release

  Szenario: Ein vorhandener Tag zeigt auf einen anderen Commit
    Angenommen der gewünschte Release-Tag zeigt auf einen anderen Commit
    Wenn die CI den Release vorbereitet
    Dann bricht sie mit einer Fehlermeldung ab und verschiebt den Tag nicht

  Szenario: Fehlgeschlagene Veröffentlichung wiederholen
    Angenommen ein Release wurde noch nicht veröffentlicht
    Wenn die CI manuell für denselben Commit gestartet wird
    Dann kann die CI den Release erneut versuchen
